package main

import (
	"fmt"
	"strconv"
	"os"

	"github.com/droundy/goopt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
)

var cluster = goopt.String([]string{"-c", "--cluster"}, "default", "ECS Cluster to query")
var region = goopt.String([]string{"-r", "--region"}, "us-east-1", "AWS Region")
var profile = goopt.String([]string{"-p", "--profile"}, "default", "AWS Profile")
var containerID = goopt.String([]string{"-i", "--containerID"}, "unknown", "Container ID")
var show = goopt.Alternatives([]string{"-s", "--show"},
	[]string{"all", "ServiceTasks", "ManualTasks", "ContainerInfo"},
	"Details to print about Cluster Services & Tasks")

var version = "0.1"
var containerInstanceMap = make(map[string]string)
var containerInstanceIDMap = make(map[string]string)
var pageNum = 0

var ecsSvc *ecs.ECS
var ec2Svc *ec2.EC2

func main() {

	// Set version
	goopt.Version = version

	// Parse any args
	goopt.Parse(nil)

	// Setup the Session
	sess := session.Must(session.NewSessionWithOptions(session.Options{
		Config: aws.Config{Region: aws.String(*region)},
		Profile: *profile,
	}))

	// Connect up to ECS
	ecsSvc = ecs.New(sess)
	// Connect up to EC2
	ec2Svc = ec2.New(sess)

	//cluster := "search-prod"
	fmt.Println("")
	fmt.Println("Using Region: " + *region)
	fmt.Println("Querying Cluster: " + *cluster)

	buildContainerMaps() // Build a map of Container ARN to DNS Names
	switch *show {
	case "all":
		listServiceTasks() // List details for Service managed tasks.
		listManualTasks()  // List details for Manual unmanaged tasks.
	case "ServiceTasks":
		listServiceTasks()
	case "ManualTasks":
		listManualTasks()
	case "ContainerInfo":
		findContainerInfo(containerID)
	default:
		panic("Unrecognized show option.")
	}
}

func die(message string) {
	fmt.Println(message)
	os.Exit(255)
}

func findContainerInfo(id *string) {
	buildContainerMaps()

	taskInput := &ecs.ListTasksInput{Cluster: aws.String(*cluster)}
	taskListResult, err := ecsSvc.ListTasks(taskInput)
	if err != nil {
		die(err.Error())
	}
	if len(taskListResult.TaskArns) >= 1 {
		oneoffTasksInput := &ecs.DescribeTasksInput{Tasks: taskListResult.TaskArns, Cluster: aws.String(*cluster)}
		taskDescResult, err := ecsSvc.DescribeTasks(oneoffTasksInput)
		if err != nil {
			die(err.Error())
		}
		if len(taskDescResult.Tasks) >= 1 {
			for _, taskInfo := range taskDescResult.Tasks {
				fmt.Println(taskInfo.Containers[0])
				/*
				if taskInfo.StartedBy == nil {
					fmt.Println("\t\tContainer " +
						*taskInfo.Containers[0].Name +
						") running on " +
						containerInstanceMap[*taskInfo.ContainerInstanceArn] + ":" +
						strconv.FormatInt(*taskInfo.Containers[0].NetworkBindings[0].HostPort, 10) +
						" (" + containerInstanceIDMap[*taskInfo.ContainerInstanceArn] + ")")
				}
				*/
			}
		}
	}
	fmt.Println("")

}

func buildContainerMaps() {
	containerListInput := &ecs.ListContainerInstancesInput{Cluster: aws.String(*cluster)}
	containerList, err := ecsSvc.ListContainerInstances(containerListInput)
	if err != nil {
		die(err.Error())
	}

	if len(containerList.ContainerInstanceArns) == 0 {
		die("No Containers in " + *cluster + " Cluster! (i.e. nothing to run containers on)")
	}

	containerDescInput := &ecs.DescribeContainerInstancesInput{
		Cluster:            aws.String(*cluster),
		ContainerInstances: containerList.ContainerInstanceArns,
	}
	containerDescriptions, err := ecsSvc.DescribeContainerInstances(containerDescInput)
	if err != nil {
		die(err.Error())
	}

	for _, desc := range containerDescriptions.ContainerInstances {

		//fmt.Println(desc)
		input := &ec2.DescribeInstancesInput{
			InstanceIds: []*string{desc.Ec2InstanceId},
		}

		instancesResult, err := ec2Svc.DescribeInstances(input)
		if err != nil {
			die(err.Error())
		}
		containerInstanceMap[*desc.ContainerInstanceArn] = *instancesResult.Reservations[0].Instances[0].PrivateDnsName
		containerInstanceIDMap[*desc.ContainerInstanceArn] = *instancesResult.Reservations[0].Instances[0].InstanceId
	}
}

func listServiceTasks() {
	fmt.Println("")
	fmt.Println("The Following Services are running on the " + *cluster + " ECS Cluster: ")
	fmt.Println("")
	input := &ecs.ListServicesInput{Cluster: aws.String(*cluster), MaxResults: aws.Int64(10)}
	res := ecsSvc.ListServicesPages(input, listTasksByService)
	if res != nil {
		die(res.Error())
	}
	fmt.Println("")
}

func listTasksByService(page *ecs.ListServicesOutput, lastPage bool) bool {
	pageNum++
	dsInput := &ecs.DescribeServicesInput{Cluster: aws.String(*cluster), Services: page.ServiceArns}
	servicesRes, err := ecsSvc.DescribeServices(dsInput)
	if err != nil {
		die(err.Error())
	}
	for _, service := range servicesRes.Services {
		fmt.Println("Service Name: " + *service.ServiceName)
		fmt.Println("\tCurrent Status: " + *service.Status)
		fmt.Println("\tLast Event: " + *service.Events[0].Message)

		taskInput := &ecs.ListTasksInput{
			Cluster:     aws.String(*cluster),
			ServiceName: aws.String(*service.ServiceName),
		}
		taskListResult, err := ecsSvc.ListTasks(taskInput)
		if err != nil {
			die(err.Error())
		}
		if len(taskListResult.TaskArns) >= 1 {
			fmt.Println("\tThere are " + strconv.Itoa(len(taskListResult.TaskArns)) + " tasks running under this service:")
			input := &ecs.DescribeTasksInput{Tasks: taskListResult.TaskArns, Cluster: aws.String(*cluster)}
			taskDescResult, err := ecsSvc.DescribeTasks(input)
			if err != nil {
				die(err.Error())
			}
			for _, taskInfo := range taskDescResult.Tasks {
				if (len(taskInfo.Containers) > 1) {
					fmt.Println("\t\t***This Task has multiple containers!***")
				}
				fmt.Println("\t\tTask: " + *taskInfo.TaskDefinitionArn)
				for i := 0; i < len(taskInfo.Containers); i++ {
					if (len(taskInfo.Containers[i].NetworkBindings) >= 1) {
						fmt.Println("\t\t\tContainer " +
							*taskInfo.Containers[i].Name +
							" running on " +
							containerInstanceMap[*taskInfo.ContainerInstanceArn] + ":" +
							strconv.FormatInt(*taskInfo.Containers[i].NetworkBindings[0].HostPort, 10) +
							" (" + containerInstanceIDMap[*taskInfo.ContainerInstanceArn] + ")")
					} else {
						fmt.Println("\t\t\tContainer " +
							*taskInfo.Containers[i].Name +
							" running on " +
							containerInstanceMap[*taskInfo.ContainerInstanceArn] +
							" (" + containerInstanceIDMap[*taskInfo.ContainerInstanceArn] + ")")
					}
				}
			}
			fmt.Println("")
		}
	}
	done := pageNum <= 50 //Assuming never more than 500 services in a cluster here...
	if done {
		fmt.Println("")
	}
	return done
}

func listManualTasks() {
	fmt.Println("")
	fmt.Println("The following manually started tasks are running on the " + *cluster + " ECS Cluster: ")
	fmt.Println("")

	taskInput := &ecs.ListTasksInput{Cluster: aws.String(*cluster)}
	taskListResult, err := ecsSvc.ListTasks(taskInput)
	if err != nil {
		die(err.Error())
	}
	if len(taskListResult.TaskArns) >= 1 {
		oneoffTasksInput := &ecs.DescribeTasksInput{Tasks: taskListResult.TaskArns, Cluster: aws.String(*cluster)}
		taskDescResult, err := ecsSvc.DescribeTasks(oneoffTasksInput)
		if err != nil {
			die(err.Error())
		}
		if len(taskDescResult.Tasks) >= 1 {
			for _, taskInfo := range taskDescResult.Tasks {
				if taskInfo.StartedBy == nil {
					if (len(taskInfo.Containers) > 1) {
						fmt.Println("\t\t***This Task has multiple containers!***")
					}
					fmt.Println("\t\tTask: " + *taskInfo.TaskDefinitionArn)
					for i := 0; i < len(taskInfo.Containers); i++ {
						if (len(taskInfo.Containers[i].NetworkBindings) >= 1) {
							fmt.Println("\t\t\tContainer " +
								*taskInfo.Containers[i].Name +
								" running on " +
								containerInstanceMap[*taskInfo.ContainerInstanceArn] + ":" +
								strconv.FormatInt(*taskInfo.Containers[i].NetworkBindings[0].HostPort, 10) +
								" (" + containerInstanceIDMap[*taskInfo.ContainerInstanceArn] + ")")
						} else {
							fmt.Println("\t\t\tContainer " +
								*taskInfo.Containers[i].Name +
								" running on " +
								containerInstanceMap[*taskInfo.ContainerInstanceArn] +
								" (" + containerInstanceIDMap[*taskInfo.ContainerInstanceArn] + ")")
						}
					}
				}
			}
		}
	}
	fmt.Println("")
}
