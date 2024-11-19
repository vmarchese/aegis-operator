/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"os"

	corev1 "k8s.io/api/core/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	annotationEgressKey   = "aegisproxy.io/egress"
	annotationIngressKey  = "aegisproxy.io/ingress"
	annotationIngressPort = "aegisproxy.io/ingress.port"
	annotationType        = "aegisproxy.io/type"
	annotationValue       = "true"

	ingressType       = "ingress"
	egressType        = "egress"
	ingressEgressType = "ingress-egress"

	aegisProxyContainerName = "aegis-proxy"
	aegisProxyImage         = "registry.localhost:5000/aegis-proxy:1.1"
	aegisIpTablesImage      = "registry.localhost:5000/aegis-iptables:1.0"

	initContainerName = "aegis-init"

	outboundPort = "3128"
	inboundPort  = "3127"
	userID       = 1137

	tokenMountPath    = "/var/run/secrets/tokens"
	tokenFile         = "token"
	expirationSeconds = 7200
)

//go:embed iptables-egress.sh
var iptablesEgressScript string

//go:embed iptables-ingress.sh
var iptablesIngressScript string

//go:embed iptables.sh
var iptablesIngressEgressScript string

type PodWebhook struct {
	decoder *admission.Decoder
}

var _ admission.CustomDefaulter = &PodWebhook{}

// InjectDecoder injects the decoder.
func (m *PodWebhook) InjectDecoder(d *admission.Decoder) error {
	fmt.Println("InjectDecoder")
	m.decoder = d
	return nil
}

// log is for logging in this package.
var podwebhooklog = logf.Log.WithName("podwebhook-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *PodWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	fmt.Println("SetupWebhookWithManager")
	return ctrl.NewWebhookManagedBy(mgr).
		For(&corev1.Pod{}).
		WithDefaulter(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate--v1-pod,mutating=true,failurePolicy=fail,sideEffects=None,groups="",resources=pods,verbs=create;update,versions=v1,name=mpodwebhook.kb.io,admissionReviewVersions=v1

func (m *PodWebhook) Handle(ctx context.Context, req admission.Request) admission.Response {
	fmt.Println("Handle")
	log := podwebhooklog.WithValues("name", req.Name)
	pod := &corev1.Pod{}
	if err := m.decoder.Decode(req, pod); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	log.Info("mutating pod", "name", pod.Name)

	return admission.Response{}
}

// Default implements admission.CustomDefaulter
func (m *PodWebhook) Default(ctx context.Context, obj runtime.Object) error {
	log := podwebhooklog.WithValues("name", obj.GetObjectKind())

	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return fmt.Errorf("expected a Pod")
	}

	proxyType := ""
	iptablesScript := ""
	mustInject := false
	port := ""

	// Check for presence of annotations
	if value, ok := pod.Annotations[annotationEgressKey]; ok && value == annotationValue {
		log.Info("egress annotation found", "name", pod.Name)
		proxyType = egressType
		iptablesScript = iptablesEgressScript
		mustInject = true
	}
	if value, ok := pod.Annotations[annotationIngressKey]; ok && value == annotationValue {
		log.Info("ingress annotation found", "name", pod.Name)
		if _port, ok := pod.Annotations[annotationIngressPort]; ok {
			port = _port
		} else {
			return fmt.Errorf("ingress port is not set")
		}
		if proxyType == egressType {
			proxyType = ingressEgressType
		} else {
			proxyType = ingressType
		}
		mustInject = true
	}
	if !mustInject {
		log.Info("no proxy type found, skipping", "name", pod.Name)
		return nil // No mutation required
	}
	log.Info("injecting proxy", "name", pod.Name, "type", proxyType)

	userIDs := fmt.Sprintf("%d", userID)
	switch proxyType {
	case egressType:
		iptablesScript = fmt.Sprintf(iptablesEgressScript, userIDs, outboundPort)
	case ingressType:
		iptablesScript = fmt.Sprintf(iptablesIngressScript, userIDs, inboundPort, port)
	case ingressEgressType:
		iptablesScript = fmt.Sprintf(iptablesIngressEgressScript, userIDs, inboundPort, outboundPort, port)
	}
	if err := m.injectProxy(pod, proxyType, iptablesScript); err != nil {
		return err
	}

	return nil
}

// injectProxy injects the proxy and init containers based on the proxy type
func (m *PodWebhook) injectProxy(pod *corev1.Pod, proxyType string, iptablesScript string) error {
	log := podwebhooklog.WithValues("name", pod.Name)
	userID := int64(userID)
	proxyContainerName := aegisProxyContainerName

	// Inject the aegis-proxy container if not already present
	if !hasContainer(pod, proxyContainerName) {
		log.Info("injecting aegis-proxy container", "name", pod.Name)
		aegisProxyContainer := corev1.Container{
			Name:            proxyContainerName,
			Image:           aegisProxyImage,
			ImagePullPolicy: corev1.PullAlways,
			SecurityContext: &corev1.SecurityContext{
				RunAsUser:  &userID,
				RunAsGroup: &userID,
			},
			Command: []string{
				"./aegisproxy",
			},
			Args: []string{
				"run",
				"--type", proxyType,
				"--inport", inboundPort,
				"--outport", outboundPort,
				"--token", fmt.Sprintf("%s%c%s", tokenMountPath, os.PathSeparator, tokenFile),
				"-vvvvv",
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "satoken",
					MountPath: tokenMountPath,
				},
			},
		}
		pod.Spec.Containers = append(pod.Spec.Containers, aegisProxyContainer)
	}

	// Inject the init container if not already present
	if !hasContainer(pod, initContainerName) {
		exp := int64(expirationSeconds)

		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: "satoken",
			VolumeSource: corev1.VolumeSource{
				Projected: &corev1.ProjectedVolumeSource{
					Sources: []corev1.VolumeProjection{
						{ServiceAccountToken: &corev1.ServiceAccountTokenProjection{
							Path:              "token",
							Audience:          "vault", //TODO: depends on the identity
							ExpirationSeconds: &exp,
						}}},
				},
			},
		})
		log.Info("injecting aegis-iptables init container", "name", pod.Name)
		initContainer := corev1.Container{
			Name:  initContainerName,
			Image: aegisIpTablesImage,
			Command: []string{
				"/bin/sh",
				"-c",
				iptablesScript,
			},
			SecurityContext: &corev1.SecurityContext{
				Capabilities: &corev1.Capabilities{
					Add: []corev1.Capability{"NET_ADMIN"},
				},
			},
		}
		pod.Spec.InitContainers = append(pod.Spec.InitContainers, initContainer)
	}
	return nil
}

func hasContainer(pod *corev1.Pod, containerName string) bool {
	for _, container := range append(pod.Spec.Containers, pod.Spec.InitContainers...) {
		if container.Name == containerName {
			return true
		}
	}
	return false
}
