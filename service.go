package main

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/types"
	"encoding/json"
	"regexp"
	"log"
)

func _developer(service *corev1.Service, c *Config) *corev1.Service {
	re := regexp.MustCompile(c.UserConfig.Developer.NamespacePattern)
	// Check Developer Namespace
	if ok := re.MatchString(service.ObjectMeta.Namespace); !ok { 
		return service
	}

	check := false

	for _, v := range c.UserConfig.Developer.Service.Type {
		if v == service.Spec.Type {
			check = true
			break
		}
	}

	if ! check {
		service.Spec.Type = "ClusterIP"
		for num, _ := range service.Spec.Ports {
			service.Spec.Ports[num].NodePort = 0
		}
	}

	return service
}

func InitializeService(service *corev1.Service, c *Config, clientset *kubernetes.Clientset) error {
	log.Println("We start initializeService")

	o, err := runtime.NewScheme().DeepCopy(service)
	if err != nil {
		log.Println(err)
		return err
	}

	initializedService := o.(*corev1.Service)

	if c.UserConfig.Developer.Enable {
		initializedService = _developer(service, c)
	}

	oldData, err := json.Marshal(service)
	if err != nil {
		return err
	}

	newData, err := json.Marshal(initializedService)
	if err != nil {
		return err
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, corev1.Service{})
	if err != nil {
		return err
	}

	_, err = clientset.CoreV1().Services(service.Namespace).Patch(service.Name, types.StrategicMergePatchType, patchBytes)
	if err != nil {
		log.Println(err)
		return nil
	}

	return nil
}