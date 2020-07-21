// Copyright (c) 2018, NVIDIA CORPORATION. All rights reserved.

package main

import (
	"github.com/golang/glog"
    "bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
        //"github.com/golang/glog"

	podresourcesapi "k8s.io/kubernetes/pkg/kubelet/apis/podresources/v1alpha1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

//const nvidiaResourceName = "nvidia.com/gpu"

type devicePodInfo struct {
	name      string
	namespace string
	container string
	node	  string
}

// Helper function that creates a map of pod info for each device
func createDevicePodMap(devicePods podresourcesapi.ListPodResourcesResponse) map[string]devicePodInfo {
	nvidiaResourceName := map[string]bool{
		"nvidia.com/gpu": true,
		"tencent.com/vcuda-core":  true,
		"tencent.com/vcuda-memory":  true,
	} 
	deviceToPodMap := make(map[string]devicePodInfo)
	for _, pod := range devicePods.GetPodResources() {
		for _, container := range pod.GetContainers() {
			for _, device := range container.GetDevices() {
                if  nvidiaResourceName[device.GetResourceName()] {
					podInfo := devicePodInfo{
						name:      pod.GetName(),
						namespace: pod.GetNamespace(),
						container: container.GetName(),
						node:	   os.Hostname()
					}
					if device.GetResourceName() == "nvidia.com/gpu" {
						for _, uuid := range device.GetDeviceIds() {
							deviceToPodMap[uuid] = podInfo
						}
					}
					else{
						uuid := getGpuUuid(podInfo)
						deviceToPodMap[uuid] = podInfo
					}
					break
				}
			}
		}
	}
	return deviceToPodMap
}

//only relevent for gpu-manager cases
func getGpuUuid(devicePodInfo podInfo) string{
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	pod, err := clientset.CoreV1().Pods(podInfo.namespace).Get(podInfo.name, metav1.GetOptions{})
	for _, container := range pod.Spec.Containers {
		if contianer.Name == podInfo.container{
			for _, envar := range container.Env {
				if envar == "NVIDIA_VISIBLE_DEVICES" {
					return envar.Value
				}
			}
		}
	}
}

func getDevicePodInfo(socket string) (map[string]devicePodInfo, error) {
	devicePods, err := getListOfPods(socket)
	if err != nil {
		return nil, fmt.Errorf("failed to get devices Pod information: %v", err)
	}
	return createDevicePodMap(*devicePods), nil

}

func addPodInfoToMetrics(dir string, srcFile string, destFile string, deviceToPodMap map[string]devicePodInfo) error {
	readFI, err := os.Open(srcFile)
	if err != nil {
		return fmt.Errorf("failed to open %s: %v", srcFile, err)
	}
	defer readFI.Close()
	reader := bufio.NewReader(readFI)

	tmpPrefix := "pod"
	tmpF, err := ioutil.TempFile(dir, tmpPrefix)
	if err != nil {
		return fmt.Errorf("error creating temp file: %v", err)
	}

	tmpFname := tmpF.Name()
	defer func() {
		tmpF.Close()
		os.Remove(tmpFname)
	}()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF && len(line) == 0 {
				return writeDestFile(tmpFname, destFile)
			}
			return fmt.Errorf("error reading %s: %v", srcFile, err)
		}

		// Skip comments and add pod info
		if string(line[0]) != "#" {
			uuid := strings.Split(strings.Split(line, ",")[1], "\"")[1]
                        gpuIndex := strings.Split(strings.Split(strings.Split(line, "{")[1], ",")[0], "\"")[1]
            glog.Infof("addPodInfoToMetrics. uuid=<%s> gpuIndex=<%v>", uuid, gpuIndex)
			if pod, exists := deviceToPodMap[uuid]; exists {
				glog.Infof("addPodInfoToMetrics. added pod name from uuid")
 				line = addPodInfoToLine(line, pod)
			}
                        if pod, exists := deviceToPodMap["nvidia" + string(gpuIndex)]; exists {
                  				glog.Infof("addPodInfoToMetrics. added pod name from gpu index")
                                line = addPodInfoToLine(line, pod)
                        }
		}

		_, err = tmpF.WriteString(line)
		if err != nil {
			return fmt.Errorf("error writing to %s: %v", tmpFname, err)
		}
	}
}

func addPodInfoToLine(originalLine string, pod devicePodInfo) string {
	splitOriginalLine := strings.Split(originalLine, "}")
        newLineWithPodName := fmt.Sprintf("%s,pod_name=\"%s\",pod_namespace=\"%s\",container_name=\"%s\",node=\"%s\"}%s", splitOriginalLine[0], pod.name, pod.namespace, pod.container, pod.node, splitOriginalLine[1])
        return newLineWithPodName
}
