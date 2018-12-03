// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/mocks"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/testing"
)

type lxdProfileSuite struct {
	testing.BaseSuite

	machineTag1 names.MachineTag
	unitTag1    names.UnitTag
	unitTag2    names.UnitTag
}

var _ = gc.Suite(&lxdProfileSuite{})

func (s *lxdProfileSuite) SetUpTest(c *gc.C) {
	s.machineTag1 = names.NewMachineTag("1")
	s.unitTag1 = names.NewUnitTag("mysql/1")
	s.unitTag2 = names.NewUnitTag("redis/1")
}

func (s *lxdProfileSuite) assertBackendApi(c *gc.C, tag names.Tag) (*common.LXDProfileAPI, *gomock.Controller, *mocks.MockLXDProfileBackend) {
	resources := common.NewResources()
	authorizer := apiservertesting.FakeAuthorizer{
		Tag: tag,
	}

	ctrl := gomock.NewController(c)
	mockBackend := mocks.NewMockLXDProfileBackend(ctrl)

	unitAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag.Id() == s.unitTag1.Id() {
				return true
			}
			return false
		}, nil
	}

	machineAuthFunc := func() (common.AuthFunc, error) {
		return func(tag names.Tag) bool {
			if tag.Id() == s.machineTag1.Id() {
				return true
			}
			return false
		}, nil
	}

	api := common.NewLXDProfileAPI(
		mockBackend, resources, authorizer, machineAuthFunc, unitAuthFunc, loggo.GetLogger("juju.apiserver.common"))
	return api, ctrl, mockBackend
}

func (s *lxdProfileSuite) TestWatchLXDProfileUpgradeNotificationsUnitTag(c *gc.C) {
	api, ctrl, mockBackend := s.assertBackendApi(c, s.unitTag1)
	defer ctrl.Finish()

	lxdProfileWatcher := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
	lxdProfileWatcher.changes <- struct{}{}

	mockMachine1 := mocks.NewMockLXDProfileMachine(ctrl)
	mockUnit1 := mocks.NewMockLXDProfileUnit(ctrl)

	mockBackend.EXPECT().Machine(s.machineTag1.Id()).Return(mockMachine1, nil)
	mockBackend.EXPECT().Unit(s.unitTag1.Id()).Return(mockUnit1, nil)
	mockMachine1.EXPECT().WatchLXDProfileUpgradeNotifications().Return(lxdProfileWatcher, nil)
	mockUnit1.EXPECT().AssignedMachineId().Return(s.machineTag1.Id(), nil)

	args := params.Entities{Entities: []params.Entity{
		{Tag: names.NewUnitTag("mysql/2").String()},
		{Tag: s.unitTag1.String()},
	}}
	watches, err := api.WatchLXDProfileUpgradeNotifications(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watches, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
			{NotifyWatcherId: "1", Error: nil},
		},
	})
}

func (s *lxdProfileSuite) TestWatchLXDProfileUpgradeNotificationsMachineTag(c *gc.C) {
	api, ctrl, mockBackend := s.assertBackendApi(c, s.machineTag1)
	defer ctrl.Finish()

	mockMachine := mocks.NewMockLXDProfileMachine(ctrl)

	lxdProfileWatcher := &mockNotifyWatcher{
		changes: make(chan struct{}, 1),
	}
	lxdProfileWatcher.changes <- struct{}{}

	mockBackend.EXPECT().Machine(s.machineTag1.Id()).Return(mockMachine, nil)
	mockMachine.EXPECT().WatchLXDProfileUpgradeNotifications().Return(lxdProfileWatcher, nil)

	watches, err := api.WatchLXDProfileUpgradeNotifications(
		params.Entities{
			Entities: []params.Entity{
				{Tag: s.machineTag1.String()},
				{Tag: names.NewMachineTag("7").String()},
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(watches, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{NotifyWatcherId: "", Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}

func (s *lxdProfileSuite) TestLXDProfileStatusUnitTag(c *gc.C) {
	api, ctrl, mockBackend := s.assertBackendApi(c, s.unitTag1)
	defer ctrl.Finish()

	mockUnit := mocks.NewMockLXDProfileUnit(ctrl)

	mockBackend.EXPECT().Unit(s.unitTag1.Id()).Return(mockUnit, nil)
	mockUnit.EXPECT().UpgradeCharmProfileStatus().Return(lxdprofile.SuccessStatus, nil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: s.unitTag1.String()},
			{Tag: names.NewUnitTag("mysql/2").String()},
		},
	}

	results, err := api.UpgradeCharmProfileUnitStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.DeepEquals, params.UpgradeCharmProfileStatusResults{
		Results: []params.UpgradeCharmProfileStatusResult{
			{Status: lxdprofile.SuccessStatus},
			{Error: &params.Error{Message: "permission denied", Code: "unauthorized access"}},
		},
	})
}
