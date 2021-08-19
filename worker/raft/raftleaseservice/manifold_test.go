// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftleaseservice

import (
	"github.com/golang/mock/gomock"
	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/core/raftlease"
	raftleasestore "github.com/juju/juju/state/raftlease"
	"github.com/juju/juju/worker/common"
	workerstate "github.com/juju/juju/worker/state"
)

var expectedInputs = []string{"agent", "auth", "mux", "raft", "state"}

type ManifoldSuite struct {
	testing.IsolationSuite

	manifold dependency.Manifold
	context  dependency.Context

	mux  *apiserverhttp.Mux
	raft RaftApplier

	apiInfo     *api.Info
	auth        *MockAuthenticator
	agent       *MockAgent
	config      *MockConfig
	worker      *MockWorker
	target      *MockNotifyTarget
	state       *MockState
	workerState *MockStateTracker
	leaseLogger *MockWriter
	logger      *MockLogger
	clock       *MockClock
	registerer  *MockRegisterer
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAPIInfo(c)

	s.startWorkerClean(c)
}

func (s *ManifoldSuite) TestStoppingWorkerReleasesState(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAPIInfo(c)

	s.worker.EXPECT().Kill()
	s.worker.EXPECT().Wait().Return(nil)

	// This is actually what we're looking for. If this doesn't get triggered,
	// then we know the state hasn't been released.
	s.workerState.EXPECT().Done().Return(nil)

	w := s.startWorkerClean(c)

	// Stopping the worker should cause the state to
	// eventually be released.
	workertest.CleanKill(c, w)
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w.(*common.CleanupWorker).Worker, gc.Equals, s.worker)
	return w
}

func (s *ManifoldSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.apiInfo = &api.Info{}
	s.agent = NewMockAgent(ctrl)
	s.config = NewMockConfig(ctrl)
	s.auth = NewMockAuthenticator(ctrl)
	s.worker = NewMockWorker(ctrl)
	s.target = NewMockNotifyTarget(ctrl)
	s.state = NewMockState(ctrl)
	s.workerState = NewMockStateTracker(ctrl)
	s.leaseLogger = NewMockWriter(ctrl)
	s.logger = NewMockLogger(ctrl)
	s.clock = NewMockClock(ctrl)
	s.registerer = NewMockRegisterer(ctrl)

	s.mux = &apiserverhttp.Mux{}
	s.raft = new(raft.Raft)

	s.context = s.newContext(nil)
	s.manifold = Manifold(ManifoldConfig{
		AgentName:         "agent",
		AuthenticatorName: "auth",
		MuxName:           "mux",
		RaftName:          "raft",
		StateName:         "state",
		NewWorker:         s.newWorker(c),
		NewTarget:         s.newTarget(c),

		// These are passed directly, rather than being engine dependencies.
		LeaseLog:             s.leaseLogger,
		Logger:               s.logger,
		Clock:                s.clock,
		PrometheusRegisterer: s.registerer,
		Path:                 "raftleaseservice/path",
		GetState:             s.getState(c),
	})

	return ctrl
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent": s.agent,
		"auth":  s.auth,
		"mux":   s.mux,
		"raft":  s.raft,
		"state": s.workerState,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newWorker(c *gc.C) func(Config) (worker.Worker, error) {
	return func(config Config) (worker.Worker, error) {
		c.Assert(config, gc.DeepEquals, Config{
			APIInfo:              s.apiInfo,
			Authenticator:        s.auth,
			Mux:                  s.mux,
			Path:                 "raftleaseservice/path",
			Raft:                 s.raft,
			Target:               s.target,
			Logger:               s.logger,
			Clock:                s.clock,
			PrometheusRegisterer: s.registerer,
		})
		return s.worker, nil
	}
}

func (s *ManifoldSuite) newTarget(c *gc.C) func(State, raftleasestore.Logger) raftlease.NotifyTarget {
	return func(State, raftleasestore.Logger) raftlease.NotifyTarget {
		return s.target
	}
}

func (s *ManifoldSuite) getState(c *gc.C) func(workerstate.StateTracker) (State, error) {
	return func(tracker workerstate.StateTracker) (State, error) {
		c.Assert(tracker, gc.Equals, s.workerState)

		return s.state, nil
	}
}

func (s *ManifoldSuite) expectAPIInfo(c *gc.C) {
	s.agent.EXPECT().CurrentConfig().Return(s.config)
	s.config.EXPECT().APIInfo().Return(s.apiInfo, true)
}