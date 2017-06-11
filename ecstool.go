package main

import (
	"fmt"
	"strconv"

	"github.com/droundy/goopt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"os"
)

var cluster = goopt.String([]string{"-c", "--cluster"}, "default", "ECS Cluster to query")
var region = goopt.String([]string{"-r", "--region"}, "us-east-1", "AWS Region")
var show = goopt.Alternatives([]string{"-s", "--show"},
	[]string{"all", "ServiceTasks", "ManualTasks"},
	"Details to print about Cluster Services & Tasks")

var version = "0.1"
var containerInstanceMap = make(map[string]string)
var pageNum = 0

var ecsSvc *ecs.ECS
var ec2Svc *ec2.EC2

func main() {

	// Setup the Session
	sess, err := session.NewSession(&aws.Config{Region: aws.String(*region)})
	if err != nil {
		panic(err)
	}

	// Set version
	goopt.Version = version

	// Parse any args
	goopt.Parse(nil)

	// Connect up to ECS
	ecsSvc = ecs.New(sess)
	// Connect up to EC2
	ec2Svc = ec2.New(sess)

	//cluster := "search-prod"
	fmt.Println("")
	fmt.Println("Using Region: " + *region)
	fmt.Println("Querying Cluster: " + *cluster)

	buildContainerMap() // Build a map of Container ARN to DNS Names
	switch *show {
	case "all":
		listServiceTasks() // List details for Service managed tasks.
		listManualTasks()  // List details for Manual unmanaged tasks.
	case "ServiceTasks":
		listServiceTasks()
	case "ManualTasks":
		listManualTasks()
	default:
		panic("Unrecognized show option.")
	}
}

func buildContainerMap() {
	containerListInput := &ecs.ListContainerInstancesInput{Cluster: aws.String(*cluster)}
	containerList, err := ecsSvc.ListContainerInstances(containerListInput)
	if err != nil {
		panic(err)
	}

	if len(containerList.ContainerInstanceArns) == 0 {
		fmt.Println("No Containers in " + *cluster + " Cluster! (i.e. nothing to run containers on)")
		os.Exit(255)
	}

	containerDescInput := &ecs.DescribeContainerInstancesInput{
		Cluster:            aws.String(*cluster),
		ContainerInstances: containerList.ContainerInstanceArns,
	}
	containerDescriptions, err := ecsSvc.DescribeContainerInstances(containerDescInput)
	if err != nil {
		panic(err)
	}

	for _, desc := range containerDescriptions.ContainerInstances {

		input := &ec2.DescribeInstancesInput{
			InstanceIds: []*string{desc.Ec2InstanceId},
		}

		instancesResult, err := ec2Svc.DescribeInstances(input)
		if err != nil {
			panic(err)
		}
		containerInstanceMap[*desc.ContainerInstanceArn] = *instancesResult.Reservations[0].Instances[0].PrivateDnsName
	}
}

func listServiceTasks() {
	fmt.Println("")
	fmt.Println("The Following Services are running on the " + *cluster + " ECS Cluster: ")
	fmt.Println("")
	input := &ecs.ListServicesInput{Cluster: aws.String(*cluster), MaxResults: aws.Int64(10)}
	res := ecsSvc.ListServicesPages(input, listTasksByService)
	if res != nil {
		panic(res)
	}
	fmt.Println("")
}

func listTasksByService(page *ecs.ListServicesOutput, lastPage bool) bool {
	pageNum++
	dsInput := &ecs.DescribeServicesInput{Cluster: aws.String(*cluster), Services: page.ServiceArns}
	servicesRes, err := ecsSvc.DescribeServices(dsInput)
	if err != nil {
		panic(err)
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
			panic(err)
		}
		if len(taskListResult.TaskArns) >= 1 {
			fmt.Println("\tThere are " + strconv.Itoa(len(taskListResult.TaskArns)) + " tasks running under this service:")
			input := &ecs.DescribeTasksInput{Tasks: taskListResult.TaskArns, Cluster: aws.String(*cluster)}
			taskDescResult, err := ecsSvc.DescribeTasks(input)
			if err != nil {
				panic(err)
			}
			for _, taskInfo := range taskDescResult.Tasks {
				fmt.Println("\t\tContainer " + *taskInfo.Containers[0].Name + " running on " + containerInstanceMap[*taskInfo.ContainerInstanceArn])
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
		panic(err)
	}
	if len(taskListResult.TaskArns) >= 1 {
		oneoffTasksInput := &ecs.DescribeTasksInput{Tasks: taskListResult.TaskArns, Cluster: aws.String(*cluster)}
		taskDescResult, err := ecsSvc.DescribeTasks(oneoffTasksInput)
		if err != nil {
			panic(err)
		}
		if len(taskDescResult.Tasks) >= 1 {
			for _, taskInfo := range taskDescResult.Tasks {
				if taskInfo.StartedBy == nil {
					fmt.Println("\t\tContainer " + *taskInfo.Containers[0].Name + " running on " + containerInstanceMap[*taskInfo.ContainerInstanceArn])
				}
			}
		}
	}
	fmt.Println("")
}
