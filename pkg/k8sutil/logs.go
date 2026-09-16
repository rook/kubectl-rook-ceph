/*
Copyright 2026 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package k8sutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rook/kubectl-rook-ceph/pkg/logging"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// DefaultMaxLogRequests bounds how many log streams are opened at once. A cluster large enough to
// exceed it is better served by a narrower target than by hundreds of connections to the apiserver.
const DefaultMaxLogRequests = 50

// logReconnectInterval is how long to wait before reconnecting a stream that ended, and how often
// to look for pods that have appeared or gone away. It is a variable so that tests do not have to
// wait it out.
var logReconnectInterval = 2 * time.Second

// LogStreamOptions describes which logs to read and where to write them. PodLogOptions and Out are
// required.
type LogStreamOptions struct {
	Namespace     string
	LabelSelector string
	// Container reads a single container, which may be an init or ephemeral container. Empty reads
	// the regular containers of every matching pod.
	Container string
	// AllContainers additionally reads the init and ephemeral containers, as `kubectl logs
	// --all-containers` does.
	AllContainers bool
	// PodLogOptions describes what to read from each stream. Its Container field is ignored: the
	// container is chosen per stream from Container and AllContainers above.
	PodLogOptions  *corev1.PodLogOptions
	MaxLogRequests int
	Out            io.Writer
}

// streamKey identifies one log stream: a single container of a single pod.
type streamKey struct {
	pod       string
	container string
}

// StreamLogs copies the logs of every container matching opts to opts.Out. Unlike WaitForPodToRun it
// does not wait for pods to be running, so logs remain reachable while a pod is crash looping or
// stuck pending.
//
// When opts.PodLogOptions.Follow is set the pods matching the selector are re-listed periodically,
// so a daemon that restarts under a new pod name keeps being followed, and each stream is
// re-established after it ends. This deliberately differs from `kubectl logs -f`, which stops as
// soon as the container it attached to exits.
//
// A pod whose log cannot be read fails on its own: the other streams are left running, and the
// error is reported once every stream has finished.
func StreamLogs(ctx context.Context, k8sclientset kubernetes.Interface, opts LogStreamOptions) error {
	if opts.MaxLogRequests <= 0 {
		opts.MaxLogRequests = DefaultMaxLogRequests
	}
	if opts.PodLogOptions == nil {
		opts.PodLogOptions = &corev1.PodLogOptions{}
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}

	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	s := &supervisor{
		k8sclientset: k8sclientset,
		opts:         opts,
		out:          &logWriter{out: opts.Out},
		active:       map[streamKey]context.CancelFunc{},
	}
	s.open = s.openPodLogs
	defer s.stop()

	follow := opts.PodLogOptions.Follow

	for first := true; ; first = false {
		targets, err := s.discover(runCtx)
		if err != nil {
			// A selector matching nothing is a mistake worth reporting immediately, but once
			// following, it only means the pods have not come back yet.
			if first || !follow {
				return err
			}
			logging.Warning("%v, retrying in %s", err, logReconnectInterval)
		} else if err := s.reconcile(runCtx, targets, first); err != nil {
			return err
		}

		// Without --follow every stream ends on its own once the log has been read.
		if !follow {
			s.wg.Wait()
			return s.err()
		}
		if !waitBeforeReconnect(runCtx) {
			s.wg.Wait()
			return s.err()
		}
	}
}

type supervisor struct {
	k8sclientset kubernetes.Interface
	opts         LogStreamOptions
	out          *logWriter
	// open is a seam: tests replace it to exercise streams that cannot be read.
	open func(ctx context.Context, podName string, opts *corev1.PodLogOptions) (io.ReadCloser, error)

	wg        sync.WaitGroup
	active    map[streamKey]context.CancelFunc
	warnedCap bool

	mu   sync.Mutex
	errs []error
}

func (s *supervisor) openPodLogs(ctx context.Context, podName string, opts *corev1.PodLogOptions) (io.ReadCloser, error) {
	return s.k8sclientset.CoreV1().Pods(s.opts.Namespace).GetLogs(podName, opts).Stream(ctx)
}

// discover lists the streams the selector currently resolves to, sorted so that the set admitted
// under --max-log-requests is stable between passes.
func (s *supervisor) discover(ctx context.Context) ([]streamKey, error) {
	pods, err := s.k8sclientset.CoreV1().Pods(s.opts.Namespace).List(ctx, metav1.ListOptions{LabelSelector: s.opts.LabelSelector})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods matching label %q in namespace %q. %w", s.opts.LabelSelector, s.opts.Namespace, err)
	}
	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no pod matching label %q found in namespace %q", s.opts.LabelSelector, s.opts.Namespace)
	}

	var keys []streamKey
	for _, pod := range pods.Items {
		for _, container := range containersToRead(&pod, s.opts.Container, s.opts.AllContainers) {
			keys = append(keys, streamKey{pod: pod.Name, container: container})
		}
	}
	if len(keys) == 0 {
		if s.opts.Container != "" {
			return nil, fmt.Errorf("no container %q in the pods matching label %q in namespace %q", s.opts.Container, s.opts.LabelSelector, s.opts.Namespace)
		}

		return nil, fmt.Errorf("no container to read in the pods matching label %q in namespace %q", s.opts.LabelSelector, s.opts.Namespace)
	}

	slices.SortFunc(keys, func(a, b streamKey) int {
		if a.pod != b.pod {
			return strings.Compare(a.pod, b.pod)
		}

		return strings.Compare(a.container, b.container)
	})

	return keys, nil
}

// containersToRead picks the containers of one pod to stream. A named container may be an init or
// ephemeral container, so that the container holding the reason a daemon never started is reachable;
// the default fan-out is the regular containers, since init container logs are complete and would
// otherwise be re-read on every reconnect.
func containersToRead(pod *corev1.Pod, container string, all bool) []string {
	var names []string
	for _, c := range pod.Spec.Containers {
		names = append(names, c.Name)
	}
	if container != "" || all {
		for _, c := range pod.Spec.InitContainers {
			names = append(names, c.Name)
		}
		for _, c := range pod.Spec.EphemeralContainers {
			names = append(names, c.Name)
		}
	}

	if container == "" {
		return names
	}
	if slices.Contains(names, container) {
		return []string{container}
	}

	return nil
}

// reconcile starts a stream for every newly discovered container and stops the streams whose pod has
// gone away. The cap is applied to newcomers only: a stream that is already running keeps its slot,
// because ending it would lose the logs it was started to watch.
func (s *supervisor) reconcile(ctx context.Context, targets []streamKey, first bool) error {
	if first && len(targets) > s.opts.MaxLogRequests {
		return fmt.Errorf("%d log streams exceed the --max-log-requests limit of %d, narrow the target or raise the limit", len(targets), s.opts.MaxLogRequests)
	}

	wanted := make(map[streamKey]bool, len(targets))
	for _, key := range targets {
		wanted[key] = true
	}
	for key, cancel := range s.active {
		if !wanted[key] {
			cancel()
			delete(s.active, key)
		}
	}

	reading := make(map[string]bool, len(s.active))
	for key := range s.active {
		reading[key.pod] = true
	}

	var admitted, starting []streamKey
	skipped := 0
	slots := s.opts.MaxLogRequests - len(s.active)
	for _, key := range targets {
		if s.active[key] != nil {
			admitted = append(admitted, key)
			continue
		}
		if slots <= 0 {
			skipped++
			continue
		}
		slots--
		admitted = append(admitted, key)
		starting = append(starting, key)
	}

	if skipped > 0 && !s.warnedCap {
		s.warnedCap = true
		logging.Warning("already reading the --max-log-requests limit of %d streams, not reading %d more", s.opts.MaxLogRequests, skipped)
	}

	// Decide the prefix before starting anything, since a stream writes as soon as it is running and
	// a target resolving to several streams must be prefixed from its first line.
	if len(admitted) > 1 {
		s.out.enablePrefix(sharesPod(admitted))
	}

	for _, key := range starting {
		// Attribution otherwise rides entirely on the prefix, which never appears for a target that
		// is only ever one pod at a time, such as the operator.
		if !first && !reading[key.pod] {
			reading[key.pod] = true
			logging.Info("following logs from pod %q", key.pod)
		}
		s.start(ctx, key, first)
	}

	return nil
}

func (s *supervisor) start(ctx context.Context, key streamKey, initial bool) {
	streamCtx, cancel := context.WithCancel(ctx)
	s.active[key] = cancel

	s.wg.Go(func() {
		defer cancel()
		s.stream(streamCtx, key, initial)
	})
}

// stream reads one container until its context is cancelled, reconnecting between attempts while
// following. A failure here is this stream's alone: the other streams keep running, and the error
// surfaces from StreamLogs once they have all finished.
func (s *supervisor) stream(ctx context.Context, key streamKey, initial bool) {
	out := s.out.stream(key)
	defer out.flush()

	follow := s.opts.PodLogOptions.Follow
	var lastRead time.Time

	for {
		opts := streamOptions(s.opts.PodLogOptions, key.container, initial, lastRead)

		opened, err := s.copyPodLogs(ctx, key.pod, opts, out)
		if opened {
			lastRead = time.Now()
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if !follow {
				s.record(err)
				return
			}
			// A pod that cannot be read yet is usually one that has not started yet, so following
			// keeps trying rather than giving up on it.
			logging.Warning("%v, reconnecting in %s", err, logReconnectInterval)
		}

		if !follow || !waitBeforeReconnect(ctx) {
			return
		}
	}
}

func (s *supervisor) record(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.errs = append(s.errs, err)
}

func (s *supervisor) err() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return errors.Join(s.errs...)
}

// stop ends every running stream and waits for it, so that nothing writes to the caller's writer
// after StreamLogs has returned.
func (s *supervisor) stop() {
	for key, cancel := range s.active {
		cancel()
		delete(s.active, key)
	}
	s.wg.Wait()
}

// sharesPod reports whether any pod contributes more than one stream, which is what decides between
// a "[pod]" and a "[pod/container]" prefix.
func sharesPod(keys []streamKey) bool {
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if seen[key.pod] {
			return true
		}
		seen[key.pod] = true
	}

	return false
}

// copyPodLogs reports whether the stream was opened, so that a caller following the logs can tell a
// stream that ended from one that never started.
func (s *supervisor) copyPodLogs(ctx context.Context, podName string, opts *corev1.PodLogOptions, out io.Writer) (bool, error) {
	stream, err := s.open(ctx, podName, opts)
	if err != nil {
		return false, fmt.Errorf("failed to get logs for pod %q. %w", podName, err)
	}
	defer stream.Close()

	if _, err := io.Copy(out, stream); err != nil {
		return true, fmt.Errorf("failed to stream logs for pod %q. %w", podName, err)
	}

	return true, nil
}

// streamOptions adapts the caller's options to one attempt at one container. --tail and --previous
// describe the first connection to a pod that was already running, so a pod that appeared later is
// read from its beginning, and reattaching to a container already read from skips what was printed
// before.
func streamOptions(opts *corev1.PodLogOptions, container string, initial bool, lastRead time.Time) *corev1.PodLogOptions {
	next := *opts
	next.Container = container

	// Until this stream has actually read something, it is still making its first connection, so
	// --tail and --previous still describe what to ask for. A retry after a pod that was not ready
	// yet must not quietly turn --previous into the running container's log.
	if initial && lastRead.IsZero() {
		return &next
	}

	next.TailLines = nil
	next.Previous = false
	if !lastRead.IsZero() {
		// A second of overlap costs a few duplicate lines and avoids dropping any to clock skew
		// between this host and the node.
		since := metav1.NewTime(lastRead.Add(-time.Second))
		next.SinceTime = &since
		next.SinceSeconds = nil
	}

	return &next
}

func waitBeforeReconnect(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(logReconnectInterval):
		return true
	}
}

// logWriter serializes the output of concurrent streams. Prefixing is sticky: once turned on it
// stays on, so that a rollout settling back to a single pod does not change the format again
// mid-stream.
type logWriter struct {
	mu       sync.Mutex
	out      io.Writer
	prefixed bool
	wide     bool
}

func (l *logWriter) enablePrefix(wide bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.prefixed = true
	if wide {
		l.wide = true
	}
}

// writeLine writes one line of one stream. A line that is not newline terminated, which only
// happens at the end of a stream, is terminated here when prefixing, so that the next stream's
// prefix starts at the beginning of a line rather than halfway along this one.
func (l *logWriter) writeLine(key streamKey, line []byte, terminated bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.prefixed {
		prefix := fmt.Sprintf("[%s] ", key.pod)
		if l.wide {
			prefix = fmt.Sprintf("[%s/%s] ", key.pod, key.container)
		}
		if _, err := io.WriteString(l.out, prefix); err != nil {
			return err
		}
	}
	if _, err := l.out.Write(line); err != nil {
		return err
	}
	if !terminated && l.prefixed {
		if _, err := io.WriteString(l.out, "\n"); err != nil {
			return err
		}
	}

	return nil
}

func (l *logWriter) stream(key streamKey) *streamWriter {
	return &streamWriter{parent: l, key: key}
}

// streamWriter buffers a stream until it has a whole line, so that concurrent streams cannot
// interleave within a line and a prefix is only ever written at the start of one.
type streamWriter struct {
	parent *logWriter
	key    streamKey
	buf    []byte
}

func (w *streamWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)

	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		if err := w.parent.writeLine(w.key, w.buf[:i+1], true); err != nil {
			return 0, err
		}
		w.buf = w.buf[i+1:]
	}

	return len(p), nil
}

// flush writes a trailing line that never ended in a newline, as the last read of a log that is not
// line terminated.
func (w *streamWriter) flush() {
	if len(w.buf) == 0 {
		return
	}
	if err := w.parent.writeLine(w.key, w.buf, false); err != nil {
		logging.Warning("failed to write logs for pod %q. %v", w.key.pod, err)
	}
	w.buf = nil
}
