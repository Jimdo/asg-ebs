package main // import "github.com/Jimdo/asg-ebs"

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"

	"gopkg.in/alecthomas/kingpin.v2"
)

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
		log.Fatal("Failed to get region from instance metadata", err)
	}
	log.Print("Setting region to " + region)
	asgEbs.Region = region

	availabilityZone, err := metadata.GetMetadata("placement/availability-zone")
	if err != nil {
		log.Fatal("Failed to get availability zone from instance metadata", err)
	}
	log.Print("Setting availability zone to " + availabilityZone)
	asgEbs.AvailabilityZone = availabilityZone

	instanceId, err := metadata.GetMetadata("instance-id")
	if err != nil {
		log.Fatal("Failed to get instance id from instance metadata", err)
	}
	log.Print("Setting instance id to " + instanceId)
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

	return nil
}

func (asgEbs *AsgEbs) makeFileSystem(attachAs string) error {
	cmd := exec.Command("/usr/sbin/mkfs.ext4", attachAs)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func (asgEbs *AsgEbs) mountVolume(attachAs string, directory string) error {
	cmd := exec.Command("/usr/sbin/mount -t ext4", attachAs, directory)
	err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
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
		directory        = kingpin.Flag("directory", "Directory where the volume will be mounted").Required().PlaceHolder("DIR").String()
		create           = kingpin.Flag("create", "Create volume if no volume is available").Bool()
		createSize       = kingpin.Flag("create-size", "The size of the created volume, in GiBs").PlaceHolder("SIZE").Int64()
		createName       = kingpin.Flag("create-name", "The name of the created volume").PlaceHolder("NAME").String()
		createVolumeType = kingpin.Flag("create-volume-type", "The volume type of the created volume. This can be `gp2` for General Purpose (SSD) volumes or `standard` for Magnetic volumes").PlaceHolder("TYPE").Enum("standard", "gp2")
		createTags       = CreateTags(kingpin.Flag("create-tags", "Tag to use for the new volume, can be specified multiple times").PlaceHolder("KEY=VALUE"))
	)

	kingpin.UsageTemplate(kingpin.CompactUsageTemplate)
	kingpin.CommandLine.Help = "Script to create, attach, format and mount an EBS Volume to an EC2 instance"
	kingpin.Parse()

	if *create {
		if *createSize == 0 {
			kingpin.Fatalf("required flag --create-size not provided")
		}
		if *createName == "" {
			kingpin.Fatalf("required flag --create-name not provided")
		}
		if *createVolumeType == "" {
			kingpin.Fatalf("required flag --create-volume-type not provided")
		}
	}

	asgEbs := NewAsgEbs()

	volumeCreated := false
	volume, err := asgEbs.findVolume(*tagKey, *tagValue)
	if err != nil {
		log.Fatal("Failed to find volumes", err)
	}

	if volume == nil {
		if *create {
			log.Print("Creating new volume")
			volume, err = asgEbs.createVolume(*createSize, *createName, *createVolumeType, *createTags)
			if err != nil {
				log.Fatal("Failed to create new volume", err)
			}
			volumeCreated = true
		} else {
			log.Print("No available volume can be found")
			os.Exit(2)
		}
	}

	log.Print("Attaching volume ", *volume)
	err = asgEbs.attachVolume(*volume, *attachAs)
	if err != nil {
		log.Fatal("Failed to attach volume", err)
	}

	if volumeCreated {
		log.Print("Creating filesystem on new volume", *attachAs)
		err = asgEbs.makeFileSystem(*attachAs)
		if err != nil {
			log.Fatal("Failed to create file system", err)
		}
	}

	log.Print("Mounting volume", *attachAs, "to", *directory)
	err = asgEbs.mountVolume(*attachAs, *directory)
	if err != nil {
		log.Fatal("Failed to mount volume", err)
	}

	os.Exit(0)
}
