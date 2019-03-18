package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"syscall"
	"time"

	"git.curoverse.com/arvados.git/sdk/go/arvados"
	"github.com/sirupsen/logrus"
)

// remoteRunner handles the starting and stopping of a crunch-run
// process on a remote machine.
type remoteRunner struct {
	uuid          string
	executor      Executor
	arvClient     *arvados.Client
	remoteUser    string
	timeoutTERM   time.Duration
	timeoutKILL   time.Duration
	timeoutSignal time.Duration
	onUnkillable  func(uuid string) // callback invoked when giving up on SIGKILL
	onKilled      func(uuid string) // callback invoked when process exits after SIGTERM/SIGKILL
	logger        logrus.FieldLogger

	stopping bool          // true if Stop() has been called
	sentKILL bool          // true if SIGKILL has been sent
	closed   chan struct{} // channel is closed if Close() has been called
}

// newRemoteRunner returns a new remoteRunner. Caller should ensure
// Close() is called to release resources.
func newRemoteRunner(uuid string, wkr *worker) *remoteRunner {
	rr := &remoteRunner{
		uuid:          uuid,
		executor:      wkr.executor,
		arvClient:     wkr.wp.arvClient,
		remoteUser:    wkr.instance.RemoteUser(),
		timeoutTERM:   wkr.wp.timeoutTERM,
		timeoutKILL:   wkr.wp.timeoutKILL,
		timeoutSignal: wkr.wp.timeoutSignal,
		onUnkillable:  wkr.onUnkillable,
		onKilled:      wkr.onKilled,
		logger:        wkr.logger.WithField("ContainerUUID", uuid),
		closed:        make(chan struct{}),
	}
	return rr
}

// Start a crunch-run process on the remote host.
//
// Start does not return any error encountered. The caller should
// assume the remote process _might_ have started, at least until it
// probes the worker and finds otherwise.
func (rr *remoteRunner) Start() {
	env := map[string]string{
		"ARVADOS_API_HOST":  rr.arvClient.APIHost,
		"ARVADOS_API_TOKEN": rr.arvClient.AuthToken,
	}
	if rr.arvClient.Insecure {
		env["ARVADOS_API_HOST_INSECURE"] = "1"
	}
	envJSON, err := json.Marshal(env)
	if err != nil {
		panic(err)
	}
	stdin := bytes.NewBuffer(envJSON)
	cmd := "crunch-run --detach --stdin-env '" + rr.uuid + "'"
	if rr.remoteUser != "root" {
		cmd = "sudo " + cmd
	}
	stdout, stderr, err := rr.executor.Execute(nil, cmd, stdin)
	if err != nil {
		rr.logger.WithField("stdout", string(stdout)).
			WithField("stderr", string(stderr)).
			WithError(err).
			Error("error starting crunch-run process")
		return
	}
	rr.logger.Info("crunch-run process started")
}

// Close abandons the remote process (if any) and releases
// resources. Close must not be called more than once.
func (rr *remoteRunner) Close() {
	close(rr.closed)
}

// Kill starts a background task to kill the remote process,
// escalating from SIGTERM to SIGKILL to onUnkillable() according to
// the configured timeouts.
//
// Once Kill has been called, calling it again has no effect.
func (rr *remoteRunner) Kill(reason string) {
	if rr.stopping {
		return
	}
	rr.stopping = true
	rr.logger.WithField("Reason", reason).Info("killing crunch-run process")
	go func() {
		termDeadline := time.Now().Add(rr.timeoutTERM)
		killDeadline := termDeadline.Add(rr.timeoutKILL)
		t := time.NewTicker(rr.timeoutSignal)
		defer t.Stop()
		for range t.C {
			switch {
			case rr.isClosed():
				return
			case time.Now().After(killDeadline):
				rr.onUnkillable(rr.uuid)
				return
			case time.Now().After(termDeadline):
				rr.sentKILL = true
				rr.kill(syscall.SIGKILL)
			default:
				rr.kill(syscall.SIGTERM)
			}
		}
	}()
}

func (rr *remoteRunner) kill(sig syscall.Signal) {
	logger := rr.logger.WithField("Signal", int(sig))
	logger.Info("sending signal")
	cmd := fmt.Sprintf("crunch-run --kill %d %s", sig, rr.uuid)
	if rr.remoteUser != "root" {
		cmd = "sudo " + cmd
	}
	stdout, stderr, err := rr.executor.Execute(nil, cmd, nil)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"stderr": string(stderr),
			"stdout": string(stdout),
			"error":  err,
		}).Info("kill failed")
		return
	}
	rr.onKilled(rr.uuid)
}

func (rr *remoteRunner) isClosed() bool {
	select {
	case <-rr.closed:
		return true
	default:
		return false
	}
}
