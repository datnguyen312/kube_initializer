package main

import (
	ext_v1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/types"
	"encoding/json"
	"regexp"
	"log"
)

func ingress_developer(ingress *ext_v1beta1.Ingress, c *Config)  *ext_v1beta1.Ingress {
	re := regexp.MustCompile(c.UserConfig.Developer.NamespacePattern)
	// Check Developer Namespace  
	if ok := re.MatchString(ingress.ObjectMeta.Namespace); !ok { 
		return ingress
	}

	if _, ok := ingress.ObjectMeta.Annotations["kubernetes.io/ingress.class"]; ! ok {
		ingress.ObjectMeta.Annotations["kubernetes.io/ingress.class"] = c.UserConfig.Developer.Ingress.Class
	} else if ingress.ObjectMeta.Annotations["kubernetes.io/ingress.class"] != c.UserConfig.Developer.Ingress.Class {
		ingress.ObjectMeta.Annotations["kubernetes.io/ingress.class"] = c.UserConfig.Developer.Ingress.Class
	} else {
		return ingress
	}

	return ingress
}

func InitializeIngress(ingress *ext_v1beta1.Ingress, c *Config, clientset *kubernetes.Clientset) error {
	log.Println("We start initializeIngress")
	
	o, err := runtime.NewScheme().DeepCopy(ingress)
	if err != nil {
		log.Println(err)
		return err
	}

	initializedIngress := o.(*ext_v1beta1.Ingress)

	if c.UserConfig.Developer.Enable {
		initializedIngress = ingress_developer(initializedIngress, c)
	}

	oldData, err := json.Marshal(ingress)
	if err != nil {
		return err
	}

	newData, err := json.Marshal(initializedIngress)
	if err != nil {
		return err
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, ext_v1beta1.Ingress{})
	if err != nil {
		return err
	}

	_, err = clientset.Extensions().Ingresses(ingress.Namespace).Patch(ingress.Name, types.StrategicMergePatchType, patchBytes)
	if err != nil {
		log.Println(err)
		return nil
	}

	return nil
}
