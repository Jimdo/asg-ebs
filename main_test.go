package main

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/mock"
)

const (
	defaultVolumeId = "vol-123456"
	defaultSnapshotId = "snap-123456"
	defaultMkfsInodeRatio int64 = 4096
)

type FakeAsgEbs struct {
	mock.Mock
	OnFindVolume               *mock.Call
	OnCreateVolume             *mock.Call
	OnWaitUntilVolumeAvailable *mock.Call
	OnAttachVolume             *mock.Call
	OnMakeFileSystem           *mock.Call
	OnMountVolume              *mock.Call
}

func NewFakeAsgEbs(cfg *Config) *FakeAsgEbs {
	fakeAsgEbs := &FakeAsgEbs{}
	return fakeAsgEbs
}

func (fakeAsgEbs *FakeAsgEbs) findVolume(tagKey string, tagValue string) (*string, error) {
	args := fakeAsgEbs.Called(tagKey, tagValue)
	vol := args.Get(0)
	switch v := vol.(type) {
	case string:
		return &v, args.Error(1)
	default:
		return nil, args.Error(1)
	}
}

func (fakeAsgEbs *FakeAsgEbs) findSnapshot(tagKey string, tagValue string) (*string, error) {
	args := fakeAsgEbs.Called(tagKey, tagValue)
	vol := args.Get(0)
	switch v := vol.(type) {
	case string:
		return &v, args.Error(1)
	default:
		return nil, args.Error(1)
	}
}

func (fakeAsgEbs *FakeAsgEbs) createVolume(createSize int64, createName string, createVolumeType string, createTags map[string]string, snapshotId *string) (*string, error) {
	args := fakeAsgEbs.Called(createSize, createName, createVolumeType, createTags, snapshotId)
	vol := args.Get(0)
	switch v := vol.(type) {
	case string:
		return &v, args.Error(1)
	default:
		return nil, args.Error(1)
	}
}

func (fakeAsgEbs *FakeAsgEbs) waitUntilVolumeAvailable(volumeId string) error {
	args := fakeAsgEbs.Called(volumeId)
	return args.Error(0)
}

func (fakeAsgEbs *FakeAsgEbs) attachVolume(volumeId string, attachAs string, deleteOnTermination bool) error {
	args := fakeAsgEbs.Called(volumeId, attachAs, deleteOnTermination)
	return args.Error(0)
}

func (fakeAsgEbs *FakeAsgEbs) makeFileSystem(device string, mkfsInodeRatio int64, volumeId string) error {
	args := fakeAsgEbs.Called(device, mkfsInodeRatio, volumeId)
	return args.Error(0)
}

func (fakeAsgEbs *FakeAsgEbs) mountVolume(device string, mountPoint string) error {
	args := fakeAsgEbs.Called(device, mountPoint)
	return args.Error(0)
}

func (fakeAsgEbs *FakeAsgEbs) checkDevice(device string) error {
	return nil
}

func (fakeAsgEbs *FakeAsgEbs) checkMountPoint(mountPoint string) error {
	return nil
}

func strPtr(str string) *string {
	return &str
}

func int64Ptr(i int64) *int64 {
	return &i
}

func intPtr(i int) *int {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

func newConfig() *Config {
	return &Config{
		tagKey:              strPtr("Name"),
		tagValue:            strPtr("my-name"),
		attachAs:            strPtr("xvdc"),
		mountPoint:          strPtr("/mnt"),
		createSize:          int64Ptr(200),
		mkfsInodeRatio:      int64Ptr(4096),
		createName:          strPtr("my-name"),
		createVolumeType:    strPtr("gp2"),
		createTags:          &map[string]string{},
		deleteOnTermination: boolPtr(true),
		snapshotName:        strPtr(""),
		maxRetries:          intPtr(1),
	}
}

func TestCreateVolumeIfNotFound(t *testing.T) {
	cfg := newConfig()
	fakeAsgEbs := NewFakeAsgEbs(cfg)

	fakeAsgEbs.
	On("findVolume", mock.AnythingOfType("string"), mock.AnythingOfType("string")).
	Return(nil, nil)
	fakeAsgEbs.
	On("createVolume", mock.AnythingOfType("int64"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("map[string]string"), mock.AnythingOfType("*string")).
	Return(defaultVolumeId, nil)
	fakeAsgEbs.
	On("waitUntilVolumeAvailable", mock.AnythingOfType("string")).
	Return(nil)
	fakeAsgEbs.
	On("attachVolume", defaultVolumeId, mock.AnythingOfType("string"), mock.AnythingOfType("bool")).
	Return(nil)
	fakeAsgEbs.
	On("makeFileSystem", mock.AnythingOfType("string"), mock.AnythingOfType("int64"), mock.AnythingOfType("string")).
	Return(nil)
	fakeAsgEbs.
	On("mountVolume", mock.AnythingOfType("string"), mock.AnythingOfType("string")).
	Return(nil)

	runAsgEbs(fakeAsgEbs, *cfg)

	fakeAsgEbs.AssertCalled(t, "findVolume", *cfg.tagKey, *cfg.tagValue)
	fakeAsgEbs.AssertCalled(t, "createVolume", *cfg.createSize, *cfg.createName, *cfg.createVolumeType, *cfg.createTags, (*string)(nil))
	fakeAsgEbs.AssertCalled(t, "waitUntilVolumeAvailable", defaultVolumeId)
	fakeAsgEbs.AssertCalled(t, "attachVolume", defaultVolumeId, *cfg.attachAs, *cfg.deleteOnTermination)
	fakeAsgEbs.AssertCalled(t, "makeFileSystem", filepath.Join("/dev", *cfg.attachAs), defaultMkfsInodeRatio, defaultVolumeId)
	fakeAsgEbs.AssertCalled(t, "mountVolume", filepath.Join("/dev", *cfg.attachAs), *cfg.mountPoint)
}

func TestNoVolumeCreationOnFoundVolume(t *testing.T) {
	cfg := newConfig()
	fakeAsgEbs := NewFakeAsgEbs(cfg)

	fakeAsgEbs.
	On("findVolume", mock.AnythingOfType("string"), mock.AnythingOfType("string")).
	Return(defaultVolumeId, nil)
	fakeAsgEbs.
	On("attachVolume", defaultVolumeId, mock.AnythingOfType("string"), mock.AnythingOfType("bool")).
	Return(nil)
	fakeAsgEbs.
	On("mountVolume", mock.AnythingOfType("string"), mock.AnythingOfType("string")).
	Return(nil)

	runAsgEbs(fakeAsgEbs, *cfg)

	fakeAsgEbs.AssertCalled(t, "findVolume", *cfg.tagKey, *cfg.tagValue)
	fakeAsgEbs.AssertNotCalled(t, "createVolume", *cfg.createSize, *cfg.createName, *cfg.createVolumeType, *cfg.createTags, (*string)(nil))
	fakeAsgEbs.AssertCalled(t, "attachVolume", defaultVolumeId, *cfg.attachAs, *cfg.deleteOnTermination)
	fakeAsgEbs.AssertNotCalled(t, "makeFileSystem", filepath.Join("/dev", *cfg.attachAs), defaultMkfsInodeRatio, defaultVolumeId)
	fakeAsgEbs.AssertCalled(t, "mountVolume", filepath.Join("/dev", *cfg.attachAs), *cfg.mountPoint)
}

func TestRetryIfVolumeCouldNotBeAttached(t *testing.T) {
	// This is testing for a race condition when somebody stole our volume.
	cfg := newConfig()
	fakeAsgEbs := NewFakeAsgEbs(cfg)

	anotherVolumeId := "vol-123457"

	fakeAsgEbs.
	On("findVolume", mock.AnythingOfType("string"), mock.AnythingOfType("string")).
	Return(defaultVolumeId, nil).Once()
	fakeAsgEbs.
	On("findVolume", mock.AnythingOfType("string"), mock.AnythingOfType("string")).
	Return(anotherVolumeId, nil)
	fakeAsgEbs.
	On("attachVolume", defaultVolumeId, mock.AnythingOfType("string"), mock.AnythingOfType("bool")).
	Return(errors.New("Already attached")).Once()
	fakeAsgEbs.
	On("attachVolume", anotherVolumeId, mock.AnythingOfType("string"), mock.AnythingOfType("bool")).
	Return(nil)
	fakeAsgEbs.
	On("mountVolume", mock.AnythingOfType("string"), mock.AnythingOfType("string")).
	Return(nil)

	runAsgEbs(fakeAsgEbs, *cfg)

	fakeAsgEbs.AssertNumberOfCalls(t, "findVolume", 2)
	fakeAsgEbs.AssertNumberOfCalls(t, "attachVolume", 2)
	fakeAsgEbs.AssertNotCalled(t, "makeFileSystem", filepath.Join("/dev", *cfg.attachAs), defaultMkfsInodeRatio, defaultVolumeId)
	fakeAsgEbs.AssertCalled(t, "mountVolume", filepath.Join("/dev", *cfg.attachAs), *cfg.mountPoint)
}

func TestCreateVolumeFromSnapshot(t *testing.T) {
	cfg := newConfig()
	cfg.snapshotName = strPtr("my-name")
	fakeAsgEbs := NewFakeAsgEbs(cfg)

	fakeAsgEbs.
	On("findSnapshot", mock.AnythingOfType("string"), mock.AnythingOfType("string")).
	Return(defaultSnapshotId, nil)
	fakeAsgEbs.
	On("createVolume", mock.AnythingOfType("int64"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("map[string]string"), mock.AnythingOfType("*string")).
	Return(defaultVolumeId, nil)
	fakeAsgEbs.
	On("waitUntilVolumeAvailable", mock.AnythingOfType("string")).
	Return(nil)
	fakeAsgEbs.
	On("attachVolume", defaultVolumeId, mock.AnythingOfType("string"), mock.AnythingOfType("bool")).
	Return(nil)
	fakeAsgEbs.
	On("mountVolume", mock.AnythingOfType("string"), mock.AnythingOfType("string")).
	Return(nil)

	runAsgEbs(fakeAsgEbs, *cfg)

	fakeAsgEbs.AssertCalled(t, "findSnapshot", "Name", *cfg.snapshotName)
	fakeAsgEbs.AssertNotCalled(t, "findVolume", *cfg.tagKey, *cfg.tagValue)
	fakeAsgEbs.AssertCalled(t, "createVolume", *cfg.createSize, *cfg.createName, *cfg.createVolumeType, *cfg.createTags, strPtr(defaultSnapshotId))
	fakeAsgEbs.AssertCalled(t, "waitUntilVolumeAvailable", defaultVolumeId)
	fakeAsgEbs.AssertCalled(t, "attachVolume", defaultVolumeId, *cfg.attachAs, *cfg.deleteOnTermination)
	fakeAsgEbs.AssertNotCalled(t, "makeFileSystem", filepath.Join("/dev", *cfg.attachAs), defaultMkfsInodeRatio, defaultVolumeId)
	fakeAsgEbs.AssertCalled(t, "mountVolume", filepath.Join("/dev", *cfg.attachAs), *cfg.mountPoint)
}

func TestCreateVolumeWhenSnapshotNotFound(t *testing.T) {
	cfg := newConfig()
	cfg.snapshotName = strPtr("my-name")
	fakeAsgEbs := NewFakeAsgEbs(cfg)

	fakeAsgEbs.
	On("findSnapshot", mock.AnythingOfType("string"), mock.AnythingOfType("string")).
	Return(nil, nil)
	fakeAsgEbs.
	On("createVolume", mock.AnythingOfType("int64"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("map[string]string"), mock.AnythingOfType("*string")).
	Return(defaultVolumeId, nil)
	fakeAsgEbs.
	On("waitUntilVolumeAvailable", mock.AnythingOfType("string")).
	Return(nil)
	fakeAsgEbs.
	On("attachVolume", defaultVolumeId, mock.AnythingOfType("string"), mock.AnythingOfType("bool")).
	Return(nil)
	fakeAsgEbs.
	On("makeFileSystem", mock.AnythingOfType("string"), mock.AnythingOfType("int64"), mock.AnythingOfType("string")).
	Return(nil)
	fakeAsgEbs.
	On("mountVolume", mock.AnythingOfType("string"), mock.AnythingOfType("string")).
	Return(nil)

	runAsgEbs(fakeAsgEbs, *cfg)

	fakeAsgEbs.AssertCalled(t, "findSnapshot", "Name", *cfg.snapshotName)
	fakeAsgEbs.AssertNotCalled(t, "findVolume", *cfg.tagKey, *cfg.tagValue)
	fakeAsgEbs.AssertCalled(t, "createVolume", *cfg.createSize, *cfg.createName, *cfg.createVolumeType, *cfg.createTags, (*string)(nil))
	fakeAsgEbs.AssertCalled(t, "waitUntilVolumeAvailable", defaultVolumeId)
	fakeAsgEbs.AssertCalled(t, "attachVolume", defaultVolumeId, *cfg.attachAs, *cfg.deleteOnTermination)
	fakeAsgEbs.AssertCalled(t, "makeFileSystem", filepath.Join("/dev", *cfg.attachAs), defaultMkfsInodeRatio, defaultVolumeId)
	fakeAsgEbs.AssertCalled(t, "mountVolume", filepath.Join("/dev", *cfg.attachAs), *cfg.mountPoint)
}
