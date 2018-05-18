package main

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/apps/v1beta1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/apimachinery/pkg/types"
	"encoding/json"
	"regexp"
	"log"
)

func deployment_dnsLocal(deployment *v1beta1.Deployment, c *Config) *v1beta1.Deployment {

	re := regexp.MustCompile(c.UserConfig.LOCAL_DNS.NamespacePattern)
	// Check Developer Namespace  
	if ok := re.MatchString(deployment.ObjectMeta.Namespace); !ok { 
		return deployment
	}

	// DNS: Append EmtyDir volume content resovl.conf to override resolv.conf
	var volume = corev1.Volume{
		Name: "resolv", 
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}

	deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, volume)
	// DNS: mount resolv volume as /etc/resolv.conf to every container
	var volumeMount = corev1.VolumeMount{
		Name: "resolv", 
		ReadOnly: true, 
		MountPath: "/etc/resolv.conf",
		SubPath: "resolv.conf",
	}

	for number, _ := range deployment.Spec.Template.Spec.Containers {
		deployment.Spec.Template.Spec.Containers[number].VolumeMounts = append(deployment.Spec.Template.Spec.Containers[number].VolumeMounts, volumeMount)
	}

	// log.Println(deployment.Spec.Template.Spec)

	// DNS: Add initcontainer to generate resolv.conf
	var initContainer = corev1.Container{
		Name: "resolv-generator",
		Image: "alpine",
		Command: []string{
			"/bin/sh",
			"-c",
			"echo \"nameserver $NODENAME\" >> /mount/resolv.conf && cat /etc/resolv.conf >> /mount/resolv.conf",
		},
		Env: []corev1.EnvVar{
			corev1.EnvVar{
				Name: "NODENAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "status.hostIP",
					},
				},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			corev1.VolumeMount{
				Name: "resolv",
				MountPath: "/mount",
			},
		},
	}

	deployment.Spec.Template.Spec.InitContainers = append(deployment.Spec.Template.Spec.InitContainers, initContainer)	

	return deployment
}

func deployment_developer(deployment *v1beta1.Deployment, c *Config) *v1beta1.Deployment {
	re := regexp.MustCompile(c.UserConfig.Developer.NamespacePattern)
	// Check Developer Namespace  
	if ok := re.MatchString(deployment.ObjectMeta.Namespace); !ok { 
		return deployment
	}

	if ok := deployment.Spec.Template.Spec.Affinity; ok != nil {
		if ok := deployment.Spec.Template.Spec.Affinity.NodeAffinity; ok != nil {
			if ok := deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution; ok != nil {
				deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = append(deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms, c.UserConfig.Developer.NodeSelectorTerms...)
			} else {
				var requiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{
					NodeSelectorTerms: c.UserConfig.Developer.NodeSelectorTerms,
				}
				deployment.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = requiredDuringSchedulingIgnoredDuringExecution
			}
		} else {
			var nodeAffinity = &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: c.UserConfig.Developer.NodeSelectorTerms,
				},
			}
			deployment.Spec.Template.Spec.Affinity.NodeAffinity = nodeAffinity
		}
	} else if len(c.UserConfig.Developer.NodeSelectorTerms) != 0 {
		var affinity = &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: c.UserConfig.Developer.NodeSelectorTerms,
				},
			},
		}
		deployment.Spec.Template.Spec.Affinity = affinity
	}

	return deployment
}

func InitializeDeployment(deployment *v1beta1.Deployment, c *Config, clientset *kubernetes.Clientset) error {
	log.Println("We start initializeDeployment")

	o, err := runtime.NewScheme().DeepCopy(deployment)
	if err != nil {
		log.Println(err)
		return err
	}

	initializedDeployment := o.(*v1beta1.Deployment)

	if c.UserConfig.Developer.Enable {
		initializedDeployment = deployment_developer(initializedDeployment, c)
	}

	log.Println(c.UserConfig.LOCAL_DNS)

	if c.UserConfig.LOCAL_DNS.Enable {
		initializedDeployment = deployment_dnsLocal(initializedDeployment, c)
	}

	oldData, err := json.Marshal(deployment)
	if err != nil {
		return err
	}

	newData, err := json.Marshal(initializedDeployment)
	if err != nil {
		return err
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, v1beta1.Deployment{})
	if err != nil {
		return err
	}

	_, err = clientset.AppsV1beta1().Deployments(deployment.Namespace).Patch(deployment.Name, types.StrategicMergePatchType, patchBytes)
	if err != nil {
		return err
	}
	return nil
}