/*
Copyright 2022.

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
package functional_test

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/openstack-k8s-operators/lib-common/modules/test/helpers"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	condition "github.com/openstack-k8s-operators/lib-common/modules/common/condition"
)

var _ = Describe("NovaAPI controller", func() {
	var namespace string
	var novaAPIName types.NamespacedName

	BeforeEach(func() {
		// NOTE(gibi): We need to create a unique namespace for each test run
		// as namespaces cannot be deleted in a locally running envtest. See
		// https://book.kubebuilder.io/reference/envtest.html#namespace-usage-limitation
		namespace = uuid.New().String()
		th.CreateNamespace(namespace)
		// We still request the delete of the Namespace to properly cleanup if
		// we run the test in an existing cluster.
		DeferCleanup(th.DeleteNamespace, namespace)
		// NOTE(gibi): ConfigMap generation looks up the local templates
		// directory via ENV, so provide it
		DeferCleanup(os.Setenv, "OPERATOR_TEMPLATES", os.Getenv("OPERATOR_TEMPLATES"))
		os.Setenv("OPERATOR_TEMPLATES", "../../templates")

		// Uncomment this if you need the full output in the logs from gomega
		// matchers
		// format.MaxLength = 0

	})

	When("a NovaAPI CR is created pointing to a non existent Secret", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(namespace, MessageBusSecretName))
			spec := GetDefaultNovaAPISpec()
			spec["customServiceConfig"] = "foo=bar"
			novaAPIName = CreateNovaAPI(namespace, spec)
			DeferCleanup(DeleteNovaAPI, novaAPIName)
		})

		It("is not Ready", func() {
			th.ExpectCondition(
				novaAPIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition, corev1.ConditionUnknown,
			)
		})

		It("has empty Status fields", func() {
			instance := GetNovaAPI(novaAPIName)
			// NOTE(gibi): Hash and Endpoints have `omitempty` tags so while
			// they are initialized to {} that value is omited from the output
			// when sent to the client. So we see nils here.
			Expect(instance.Status.Hash).To(BeEmpty())
			Expect(instance.Status.APIEndpoints).To(BeEmpty())
			Expect(instance.Status.ReadyCount).To(Equal(int32(0)))
			Expect(instance.Status.ServiceID).To(Equal(""))
		})

		It("is missing the secret", func() {
			th.ExpectConditionWithDetails(
				novaAPIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"Input data resources missing: secret/test-secret",
			)
		})

		When("an unrealated Secret is created the CR state does not change", func() {
			BeforeEach(func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "not-relevant-secret",
						Namespace: namespace,
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				DeferCleanup(k8sClient.Delete, ctx, secret)
			})

			It("is not Ready", func() {
				th.ExpectCondition(
					novaAPIName,
					ConditionGetterFunc(NovaAPIConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionUnknown,
				)
			})

			It("is missing the secret", func() {
				th.ExpectConditionWithDetails(
					novaAPIName,
					ConditionGetterFunc(NovaAPIConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					"Input data resources missing: secret/test-secret",
				)
			})
		})

		When("the Secret is created but some fields are missing", func() {
			BeforeEach(func() {
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      SecretName,
						Namespace: namespace,
					},
					Data: map[string][]byte{
						"NovaPassword": []byte("12345678"),
					},
				}
				Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
				DeferCleanup(k8sClient.Delete, ctx, secret)
			})

			It("is not Ready", func() {
				th.ExpectCondition(
					novaAPIName,
					ConditionGetterFunc(NovaAPIConditionGetter),
					condition.ReadyCondition,
					corev1.ConditionUnknown,
				)
			})

			It("reports that the inputes are not ready", func() {
				th.ExpectCondition(
					novaAPIName,
					ConditionGetterFunc(NovaAPIConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionFalse,
				)
			})
		})

		When("the Secret is created with all the expected fields", func() {
			BeforeEach(func() {
				DeferCleanup(
					k8sClient.Delete, ctx, CreateNovaAPISecret(namespace, SecretName))
			})

			It("reports that input is ready", func() {
				th.ExpectCondition(
					novaAPIName,
					ConditionGetterFunc(NovaAPIConditionGetter),
					condition.InputReadyCondition,
					corev1.ConditionTrue,
				)
			})

			It("generated configs successfully", func() {
				// NOTE(gibi): NovaAPI has no external dependency right now to
				// generate the configs.
				th.ExpectCondition(
					novaAPIName,
					ConditionGetterFunc(NovaAPIConditionGetter),
					condition.ServiceConfigReadyCondition,
					corev1.ConditionTrue,
				)

				configDataMap := th.GetConfigMap(
					types.NamespacedName{
						Namespace: namespace,
						Name:      fmt.Sprintf("%s-config-data", novaAPIName.Name),
					},
				)
				Expect(configDataMap).ShouldNot(BeNil())
				Expect(configDataMap.Data).Should(HaveKey("01-nova.conf"))
				Expect(configDataMap.Data).Should(
					HaveKeyWithValue("01-nova.conf",
						ContainSubstring("transport_url=rabbit://fake")))
				Expect(configDataMap.Data).Should(
					HaveKeyWithValue("03-nova-override.conf", "foo=bar"))
			})

			It("stored the input hash in the Status", func() {
				Eventually(func(g Gomega) {
					novaAPI := GetNovaAPI(novaAPIName)
					g.Expect(novaAPI.Status.Hash).Should(HaveKeyWithValue("input", Not(BeEmpty())))
				}, timeout, interval).Should(Succeed())

			})

			When("the NovaAPI is deleted", func() {
				It("deletes the generated ConfigMaps", func() {
					th.ExpectCondition(
						novaAPIName,
						ConditionGetterFunc(NovaAPIConditionGetter),
						condition.ServiceConfigReadyCondition,
						corev1.ConditionTrue,
					)

					DeleteNovaAPI(novaAPIName)

					Eventually(func() []corev1.ConfigMap {
						return th.ListConfigMaps(novaAPIName.Name).Items
					}, timeout, interval).Should(BeEmpty())
				})
			})
		})
	})

	When("NovAPI is created with a proper Secret", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaAPISecret(namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(namespace, MessageBusSecretName))

			novaAPIName = CreateNovaAPI(namespace, GetDefaultNovaAPISpec())
			DeferCleanup(DeleteNovaAPI, novaAPIName)
		})

		It(" reports input ready", func() {
			th.ExpectCondition(
				novaAPIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.InputReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("NovAPI is created", func() {
		var statefulSetName types.NamespacedName

		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaAPISecret(namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(namespace, MessageBusSecretName))

			novaAPIName = CreateNovaAPI(namespace, GetDefaultNovaAPISpec())
			DeferCleanup(DeleteNovaAPI, novaAPIName)

			th.ExpectCondition(
				novaAPIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ServiceConfigReadyCondition,
				corev1.ConditionTrue,
			)

			statefulSetName = types.NamespacedName{
				Namespace: namespace,
				Name:      novaAPIName.Name,
			}
		})

		It("creates a StatefulSet for the nova-api service", func() {
			th.ExpectConditionWithDetails(
				novaAPIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.DeploymentReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				condition.DeploymentReadyRunningMessage,
			)

			ss := th.GetStatefulSet(statefulSetName)
			Expect(int(*ss.Spec.Replicas)).To(Equal(1))
			Expect(ss.Spec.Template.Spec.Volumes).To(HaveLen(2))
			Expect(ss.Spec.Template.Spec.Containers).To(HaveLen(2))

			container := ss.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(2))
			Expect(container.Image).To(Equal(ContainerImage))

			Expect(container.LivenessProbe.HTTPGet.Port.IntVal).To(Equal(int32(8774)))
			Expect(container.ReadinessProbe.HTTPGet.Port.IntVal).To(Equal(int32(8774)))

		})

		When("the StatefulSet has at least one Replica ready", func() {
			BeforeEach(func() {
				th.ExpectConditionWithDetails(
					novaAPIName,
					ConditionGetterFunc(NovaAPIConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionFalse,
					condition.RequestedReason,
					condition.DeploymentReadyRunningMessage,
				)
				th.SimulateStatefulSetReplicaReady(statefulSetName)
			})

			It("reports that the StatefulSet is ready", func() {
				th.ExpectCondition(
					novaAPIName,
					ConditionGetterFunc(NovaAPIConditionGetter),
					condition.DeploymentReadyCondition,
					corev1.ConditionTrue,
				)

				novaAPI := GetNovaAPI(novaAPIName)
				Expect(novaAPI.Status.ReadyCount).To(BeNumerically(">", 0))
			})
		})

		It("exposes the service", func() {
			th.SimulateStatefulSetReplicaReady(statefulSetName)
			th.ExpectCondition(
				novaAPIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ExposeServiceReadyCondition,
				corev1.ConditionTrue,
			)
			GetService(types.NamespacedName{Namespace: namespace, Name: "nova-public"})
			GetService(types.NamespacedName{Namespace: namespace, Name: "nova-internal"})
			AssertRouteExists(types.NamespacedName{Namespace: namespace, Name: "nova-public"})
		})

		It("creates KeystoneEndpoint", func() {
			th.SimulateStatefulSetReplicaReady(statefulSetName)
			th.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: namespace, Name: "nova"})

			keystoneEndpoint := th.GetKeystoneEndpoint(types.NamespacedName{Namespace: namespace, Name: "nova"})
			endpoints := keystoneEndpoint.Spec.Endpoints
			Expect(endpoints).To(HaveKeyWithValue("public", "http:/v2.1"))
			Expect(endpoints).To(HaveKeyWithValue("internal", "http://nova-internal."+namespace+".svc:8774/v2.1"))

			th.ExpectCondition(
				novaAPIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.KeystoneEndpointReadyCondition,
				corev1.ConditionTrue,
			)
		})

		It("is Ready", func() {
			th.SimulateStatefulSetReplicaReady(statefulSetName)
			th.SimulateKeystoneEndpointReady(types.NamespacedName{Namespace: namespace, Name: "nova"})

			th.ExpectCondition(
				novaAPIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
	When("NovaAPI CR instance is deleted", func() {
		var statefulSetName types.NamespacedName
		var keystoneEndpointName types.NamespacedName
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaAPISecret(namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(namespace, MessageBusSecretName))

			novaAPIName = CreateNovaAPI(namespace, GetDefaultNovaAPISpec())
			DeferCleanup(DeleteNovaAPI, novaAPIName)
			statefulSetName = types.NamespacedName{Namespace: namespace, Name: novaAPIName.Name}
			keystoneEndpointName = types.NamespacedName{Namespace: namespace, Name: "nova"}
		})

		It("removes the finalizer from KeystoneEndpoint", func() {
			th.SimulateStatefulSetReplicaReady(statefulSetName)
			th.SimulateKeystoneEndpointReady(keystoneEndpointName)
			th.ExpectCondition(
				novaAPIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)

			endpoint := th.GetKeystoneEndpoint(keystoneEndpointName)
			Expect(endpoint.Finalizers).To(ContainElement("NovaAPI"))

			DeleteNovaAPI(novaAPIName)
			endpoint = th.GetKeystoneEndpoint(keystoneEndpointName)
			Expect(endpoint.Finalizers).NotTo(ContainElement("NovaAPI"))
		})
	})
	When("NovaAPI is created with networkAttachments", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaAPISecret(namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(namespace, MessageBusSecretName))

			spec := GetDefaultNovaAPISpec()
			spec["networkAttachments"] = []string{"internalapi"}
			novaAPIName = CreateNovaAPI(namespace, spec)
			DeferCleanup(DeleteNovaAPI, novaAPIName)
		})

		It("reports that the definition is missing", func() {
			th.ExpectConditionWithDetails(
				novaAPIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.RequestedReason,
				"NetworkAttachment resources missing: internalapi",
			)
			th.ExpectCondition(
				novaAPIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionUnknown,
			)
		})
		It("reports that network attachment is missing", func() {
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(DeleteNetworkAttachmentDefinition, internalAPINADName)

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      novaAPIName.Name,
			}
			ss := th.GetStatefulSet(statefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:      "internalapi",
						Namespace: namespace,
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We don't add network attachment status annotations to the Pods
			// to simulate that the network attachments are missing.
			SimulateStatefulSetReplicaReadyWithPods(statefulSetName, map[string][]string{})

			th.ExpectConditionWithDetails(
				novaAPIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occured "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports that an IP is missing", func() {
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(DeleteNetworkAttachmentDefinition, internalAPINADName)

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      novaAPIName.Name,
			}
			ss := th.GetStatefulSet(statefulSetName)

			expectedAnnotation, err := json.Marshal(
				[]networkv1.NetworkSelectionElement{
					{
						Name:      "internalapi",
						Namespace: namespace,
					}})
			Expect(err).ShouldNot(HaveOccurred())
			Expect(ss.Spec.Template.ObjectMeta.Annotations).To(
				HaveKeyWithValue("k8s.v1.cni.cncf.io/networks", string(expectedAnnotation)),
			)

			// We simulat that there is no IP associated with the internalapi
			// network attachment
			SimulateStatefulSetReplicaReadyWithPods(
				statefulSetName,
				map[string][]string{namespace + "/internalapi": {}},
			)

			th.ExpectConditionWithDetails(
				novaAPIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionFalse,
				condition.ErrorReason,
				"NetworkAttachments error occured "+
					"not all pods have interfaces with ips as configured in NetworkAttachments: [internalapi]",
			)
		})
		It("reports NetworkAttachmentsReady if the Pods got the proper annotiations", func() {
			internalAPINADName := types.NamespacedName{Namespace: namespace, Name: "internalapi"}
			CreateNetworkAttachmentDefinition(internalAPINADName)
			DeferCleanup(DeleteNetworkAttachmentDefinition, internalAPINADName)

			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      novaAPIName.Name,
			}
			SimulateStatefulSetReplicaReadyWithPods(
				statefulSetName,
				map[string][]string{namespace + "/internalapi": {"10.0.0.1"}},
			)

			th.ExpectCondition(
				novaAPIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.NetworkAttachmentsReadyCondition,
				corev1.ConditionTrue,
			)

			Eventually(func(g Gomega) {
				novaAPI := GetNovaAPI(novaAPIName)
				g.Expect(novaAPI.Status.NetworkAttachments).To(
					Equal(map[string][]string{namespace + "/internalapi": {"10.0.0.1"}}))

			}, timeout, interval).Should(Succeed())

			keystoneEndpointName := types.NamespacedName{Namespace: namespace, Name: "nova"}
			th.SimulateKeystoneEndpointReady(keystoneEndpointName)

			th.ExpectCondition(
				novaAPIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})

	When("NovaAPI is created with externalEndpoints", func() {
		BeforeEach(func() {
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaAPISecret(namespace, SecretName))
			DeferCleanup(
				k8sClient.Delete, ctx, CreateNovaMessageBusSecret(namespace, MessageBusSecretName))

			spec := GetDefaultNovaAPISpec()
			// NOTE(gibi): We need to create the data as raw list of maps
			// to allow defaulting to happen according to the kubebuilder
			// definitions
			var externalEndpoints []interface{}
			externalEndpoints = append(
				externalEndpoints, map[string]interface{}{
					"endpoint":        "internal",
					"ipAddressPool":   "osp-internalapi",
					"loadBalancerIPs": []string{"internal-lb-ip-1", "internal-lb-ip-2"},
				},
			)
			spec["externalEndpoints"] = externalEndpoints

			novaAPIName = CreateNovaAPI(namespace, spec)
			DeferCleanup(DeleteNovaAPI, novaAPIName)
		})

		It("creates MetalLB service", func() {
			statefulSetName := types.NamespacedName{
				Namespace: namespace,
				Name:      novaAPIName.Name,
			}
			th.SimulateStatefulSetReplicaReady(statefulSetName)

			keystoneEndpointName := types.NamespacedName{Namespace: namespace, Name: "nova"}
			th.SimulateKeystoneEndpointReady(keystoneEndpointName)

			// As the internal enpoint is configured in ExternalEndpoints it does not
			// get a Route but a Service with MetalLB annotations instead
			service := GetService(types.NamespacedName{Namespace: namespace, Name: "nova-internal"})
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/address-pool", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/allow-shared-ip", "osp-internalapi"))
			Expect(service.Annotations).To(
				HaveKeyWithValue("metallb.universe.tf/loadBalancerIPs", "internal-lb-ip-1,internal-lb-ip-2"))
			AssertRouteNotExists(types.NamespacedName{Namespace: namespace, Name: "nova-internal"})

			// As the public endpoint is not mentioned in the ExternalEndpoints a generic Service and
			// a Route is created
			service = GetService(types.NamespacedName{Namespace: namespace, Name: "nova-public"})
			Expect(service.Annotations).NotTo(HaveKey("metallb.universe.tf/address-pool"))
			Expect(service.Annotations).NotTo(HaveKey("metallb.universe.tf/allow-shared-ip"))
			Expect(service.Annotations).NotTo(HaveKey("metallb.universe.tf/loadBalancerIPs"))
			AssertRouteExists(types.NamespacedName{Namespace: namespace, Name: "nova-public"})

			th.ExpectCondition(
				novaAPIName,
				ConditionGetterFunc(NovaAPIConditionGetter),
				condition.ReadyCondition,
				corev1.ConditionTrue,
			)
		})
	})
})
