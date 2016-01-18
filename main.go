package main // import "github.com/Jimdo/asg-ebs"

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"

	"gopkg.in/alecthomas/kingpin.v2"

	log "github.com/Sirupsen/logrus"
)

func waitForFile(file string, timeout time.Duration) error {
	startTime := time.Now()
	if _, err := os.Stat(file); err == nil {
		return nil
	}
	newTimeout := timeout - time.Since(startTime)
	if newTimeout > 0 {
		return waitForFile(file, newTimeout)
	} else {
		return errors.New("File " + file + " not found")
	}
}

func run(cmd string, args ...string) error {
	log.WithFields(log.Fields{"cmd": cmd, "args": args}).Info("Running command")
	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		log.WithFields(log.Fields{"cmd": cmd, "args": args, "err": err, "out": out}).Info("Error running command")
		return err
	}
	return nil
}

type AsgEbs struct {
	AwsConfig        *aws.Config
	Region           string
	AvailabilityZone string
	InstanceId       string
}

func NewAsgEbs() *AsgEbs {
	asgEbs := &AsgEbs{}

	metadata := ec2metadata.New(session.New())

	region, err := metadata.Region()
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Fatal("Failed to get region from instance metadata")
	}
	log.WithFields(log.Fields{"region": region}).Info("Setting region")
	asgEbs.Region = region

	availabilityZone, err := metadata.GetMetadata("placement/availability-zone")
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Fatal("Failed to get availability zone from instance metadata")
	}
	log.WithFields(log.Fields{"az": availabilityZone}).Info("Setting availability zone")
	asgEbs.AvailabilityZone = availabilityZone

	instanceId, err := metadata.GetMetadata("instance-id")
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Fatal("Failed to get instance id from instance metadata")
	}
	log.WithFields(log.Fields{"instance_id": instanceId}).Info("Setting instance id")
	asgEbs.InstanceId = instanceId

	asgEbs.AwsConfig = aws.NewConfig().
		WithRegion(region).
		WithCredentials(ec2rolecreds.NewCredentials(session.New()))

	return asgEbs
}

func (asgEbs *AsgEbs) findVolume(tagKey string, tagValue string) (*string, error) {
	svc := ec2.New(session.New(asgEbs.AwsConfig))

	params := &ec2.DescribeVolumesInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("tag:" + tagKey),
				Values: []*string{
					aws.String(tagValue),
				},
			},
			{
				Name: aws.String("status"),
				Values: []*string{
					aws.String("available"),
				},
			},
			{
				Name: aws.String("availability-zone"),
				Values: []*string{
					aws.String(asgEbs.AvailabilityZone),
				},
			},
		},
	}

	describeVolumesOutput, err := svc.DescribeVolumes(params)
	if err != nil {
		return nil, err
	}
	if len(describeVolumesOutput.Volumes) == 0 {
		return nil, nil
	}
	return describeVolumesOutput.Volumes[0].VolumeId, nil
}

func (asgEbs *AsgEbs) createVolume(createSize int64, createName string, createVolumeType string, createTags map[string]string) (*string, error) {
	svc := ec2.New(session.New(asgEbs.AwsConfig))

	createVolumeInput := &ec2.CreateVolumeInput{
		AvailabilityZone: &asgEbs.AvailabilityZone,
		Size:             aws.Int64(createSize),
		VolumeType:       aws.String(createVolumeType),
	}
	vol, err := svc.CreateVolume(createVolumeInput)
	if err != nil {
		return nil, err
	}
	tags := []*ec2.Tag{
		{
			Key:   aws.String("Name"),
			Value: aws.String(createName),
		},
	}
	for k, v := range createTags {
		tags = append(tags,
			&ec2.Tag{
				Key:   aws.String(k),
				Value: aws.String(v),
			},
		)
	}

	createTagsInput := &ec2.CreateTagsInput{
		Resources: []*string{vol.VolumeId},
		Tags:      tags,
	}
	_, err = svc.CreateTags(createTagsInput)
	if err != nil {
		return vol.VolumeId, err
	}

	describeVolumeInput := &ec2.DescribeVolumesInput{
		VolumeIds: []*string{vol.VolumeId},
	}
	err = svc.WaitUntilVolumeAvailable(describeVolumeInput)
	if err != nil {
		return vol.VolumeId, err
	}
	return vol.VolumeId, nil
}

func (asgEbs *AsgEbs) attachVolume(volumeId string, attachAs string) error {
	svc := ec2.New(session.New(asgEbs.AwsConfig))

	attachVolumeInput := &ec2.AttachVolumeInput{
		VolumeId:   aws.String(volumeId),
		Device:     aws.String(attachAs),
		InstanceId: aws.String(asgEbs.InstanceId),
	}
	_, err := svc.AttachVolume(attachVolumeInput)
	if err != nil {
		return err
	}

	describeVolumeInput := &ec2.DescribeVolumesInput{
		VolumeIds: []*string{aws.String(volumeId)},
	}
	err = svc.WaitUntilVolumeInUse(describeVolumeInput)
	if err != nil {
		return err
	}

	err = waitForFile("/dev/"+attachAs, 5*time.Second)
	if err != nil {
		return err
	}

	return nil
}

func (asgEbs *AsgEbs) makeFileSystem(device string) error {
	return run("/usr/sbin/mkfs.ext4", device)
}

func (asgEbs *AsgEbs) mountVolume(device string, mountPoint string) error {
	err := os.MkdirAll(mountPoint, 0755)
	if err != nil {
		return err
	}
	return run("/bin/mount", "-t ext4", device, mountPoint)
}

type CreateTagsValue map[string]string

func (v CreateTagsValue) Set(str string) error {
	parts := strings.SplitN(str, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("expected KEY=VALUE got '%s'", str)
	}
	key := parts[0]
	value := parts[1]
	v[key] = value
	return nil
}

func (v CreateTagsValue) String() string {
	return ""
}

func CreateTags(s kingpin.Settings) (target *map[string]string) {
	newMap := make(map[string]string)
	target = &newMap
	s.SetValue((*CreateTagsValue)(target))
	return
}

func main() {
	var (
		tagKey           = kingpin.Flag("tag-key", "The tag key to search for").Required().PlaceHolder("KEY").String()
		tagValue         = kingpin.Flag("tag-value", "The tag value to search for").Required().PlaceHolder("VALUE").String()
		attachAs         = kingpin.Flag("attach-as", "device name e.g. xvdb").Required().PlaceHolder("DEVICE").String()
		mountPoint       = kingpin.Flag("mount-point", "Directory where the volume will be mounted").Required().PlaceHolder("DIR").String()
		createSize       = kingpin.Flag("create-size", "The size of the created volume, in GiBs").Required().PlaceHolder("SIZE").Int64()
		createName       = kingpin.Flag("create-name", "The name of the created volume").Required().PlaceHolder("NAME").String()
		createVolumeType = kingpin.Flag("create-volume-type", "The volume type of the created volume. This can be `gp2` for General Purpose (SSD) volumes or `standard` for Magnetic volumes").Required().PlaceHolder("TYPE").Enum("standard", "gp2")
		createTags       = CreateTags(kingpin.Flag("create-tags", "Tag to use for the new volume, can be specified multiple times").PlaceHolder("KEY=VALUE"))
	)

	kingpin.UsageTemplate(kingpin.CompactUsageTemplate)
	kingpin.CommandLine.Help = "Script to create, attach, format and mount an EBS Volume to an EC2 instance"
	kingpin.Parse()

	asgEbs := NewAsgEbs()

	volumeCreated := false
	attachAsDevice := "/dev/" + *attachAs

	volume, err := asgEbs.findVolume(*tagKey, *tagValue)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Fatal("Failed to find volume")
	}

	if volume == nil {
		log.Info("Creating new volume")
		volume, err = asgEbs.createVolume(*createSize, *createName, *createVolumeType, *createTags)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Fatal("Failed to create new volume")
		}
		volumeCreated = true
	}

	log.WithFields(log.Fields{"volume": *volume, "device": attachAsDevice}).Info("Attaching volume")
	err = asgEbs.attachVolume(*volume, *attachAs)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Fatal("Failed to attach volume")
	}

	if volumeCreated {
		log.WithFields(log.Fields{"device": attachAsDevice}).Info("Creating file system on new volume")
		err = asgEbs.makeFileSystem(attachAsDevice)
		if err != nil {
			log.WithFields(log.Fields{"error": err}).Fatal("Failed to create file system")
		}
	}

	log.WithFields(log.Fields{"device": attachAsDevice, "mount_point": *mountPoint}).Info("Mounting volume")
	err = asgEbs.mountVolume(attachAsDevice, *mountPoint)
	if err != nil {
		log.WithFields(log.Fields{"error": err}).Fatal("Failed to mount volume")
	}
}
