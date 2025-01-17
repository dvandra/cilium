// Copyright 2018-2019 Authors of Cilium
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package k8sTest

import (
	"fmt"
	"time"

	. "github.com/cilium/cilium/test/ginkgo-ext"
	"github.com/cilium/cilium/test/helpers"
	. "github.com/onsi/gomega"
)

var longTimeout = 10 * time.Minute

// ExpectKubeDNSReady is a wrapper around helpers/WaitKubeDNS. It asserts that
// the error returned by that function is nil.
func ExpectKubeDNSReady(vm *helpers.Kubectl) {
	err := vm.WaitKubeDNS()
	ExpectWithOffset(1, err).Should(BeNil(), "kube-dns was not able to get into ready state")

	err = vm.KubeDNSPreFlightCheck()
	ExpectWithOffset(1, err).Should(BeNil(), "kube-dns service not ready")
}

// ExpectCiliumReady is a wrapper around helpers/WaitForPods. It asserts that
// the error returned by that function is nil.
func ExpectCiliumReady(vm *helpers.Kubectl) {
	ExpectCiliumRunning(vm)
	err := vm.CiliumPreFlightCheck()
	ExpectWithOffset(1, err).Should(BeNil(), "cilium pre-flight checks failed")
}

// ExpectCiliumOperatorReady is a wrapper around helpers/WaitForPods. It asserts that
// the error returned by that function is nil.
func ExpectCiliumOperatorReady(vm *helpers.Kubectl) {
	ExpectDeployReady(vm, "kube-system", "cilium-operator", longTimeout)
}

// ExpectCiliumRunning is a wrapper around helpers/WaitForPodsRunning. It
// asserts the cilium pods are running on all nodes.
func ExpectCiliumRunning(vm *helpers.Kubectl) {
	err := vm.WaitforDaemonSetReady(helpers.KubeSystemNamespace, "cilium", longTimeout)
	ExpectWithOffset(1, err).Should(BeNil(), "cilium was not able to get into ready state")
}

// ExpectAllPodsTerminated is a wrapper around helpers/WaitCleanAllTerminatingPods.
// It asserts that the error returned by that function is nil.
func ExpectAllPodsTerminated(vm *helpers.Kubectl) {
	err := vm.WaitCleanAllTerminatingPods(helpers.HelperTimeout)
	ExpectWithOffset(1, err).To(BeNil(), "terminating containers are not deleted after timeout")
}

// ExpectETCDOperatorReady is a wrapper around helpers/ExpectDeployReady. It asserts
// the error returned by that function is nil.
func ExpectETCDOperatorReady(vm *helpers.Kubectl) {
	// Etcd operator creates 5 nodes (1 cilium-etcd-operator + 1 etcd-operator + 3 etcd nodes),
	// the new pods are added when the previous is ready,
	// so we need to wait until 5 pods are in ready state.
	// This is to avoid cases where a few pods are ready, but the
	// new one is not created yet.
	By("Waiting for all etcd-operator pods are ready")
	ExpectDeployReady(vm, "kube-system", "cilium-etcd-operator", longTimeout)
}

// ExpectCiliumPreFlightInstallReady is a wrapper around helpers/WaitForNPods.
// It asserts the error returned by that function is nil.
func ExpectCiliumPreFlightInstallReady(vm *helpers.Kubectl) {
	By("Waiting for all cilium pre-flight pods to be ready")

	err := vm.WaitforDaemonSetReady(helpers.KubeSystemNamespace, "cilium-pre-flight-check", longTimeout)
	warningMessage := ""
	if err != nil {
		res := vm.Exec(fmt.Sprintf(
			"%s -n %s get pods -l k8s-app=cilium-pre-flight-check",
			helpers.KubectlCmd, helpers.KubeSystemNamespace))
		warningMessage = res.Output().String()
	}
	Expect(err).To(BeNil(), "cilium pre-flight check is not ready after timeout, pods status:\n %s", warningMessage)
}

// ExpectDaemonSetReady is a wrapper around helpers.WaitforDaemonSetReady
// It asserts that the error returned by that function is nil, indicating that
// the daemonset's pods were ready within timeout.
func ExpectDaemonSetReady(vm *helpers.Kubectl, namespace, name string, timeout time.Duration) {
	err := vm.WaitforDaemonSetReady(namespace, name, timeout)
	Expect(err).To(BeNil(), "DaemonSet %s/%s not ready after timeout:\n %s", namespace, name, err)
}

// ExpectDeployReady is a wrapper around helpers.WaitforDeployReady
// It asserts that the error returned by that function is nil, indicating that
// the deploy's pods were ready within timeout.
func ExpectDeployReady(vm *helpers.Kubectl, namespace, name string, timeout time.Duration) {
	err := vm.WaitforDeployReady(namespace, name, timeout)
	Expect(err).To(BeNil(), "Deploy %s/%s not ready after timeout:\n %s", namespace, name, err)
}

// ProvisionInfraPods deploys DNS, etcd-operator, and cilium into the kubernetes
// cluster of which vm is a member.
func ProvisionInfraPods(vm *helpers.Kubectl) {
	By("Installing DNS Deployment")
	_ = vm.Apply(helpers.DNSDeployment())

	By("Deploying etcd-operator")
	err := vm.DeployETCDOperator()
	Expect(err).To(BeNil(), "Unable to deploy etcd operator")

	By("Installing Cilium")
	err = vm.CiliumInstall(helpers.CiliumDefaultDSPatch, helpers.CiliumConfigMapPatch)
	Expect(err).To(BeNil(), "Cilium cannot be installed")

	By("Installing Cilium-Operator")
	operatorIsInstalled, err := vm.CiliumOperatorInstall("head")
	Expect(err).To(BeNil(), "Cannot install Cilium Operator")

	switch helpers.GetCurrentIntegration() {
	case helpers.CIIntegrationFlannel:
		ExpectCiliumRunning(vm)
		vm.Apply(helpers.GetFilePath("../examples/kubernetes/addons/flannel/flannel.yaml"))
	default:
	}

	ExpectETCDOperatorReady(vm)
	Expect(vm.WaitKubeDNS()).To(BeNil(), "KubeDNS is not ready after timeout")
	ExpectCiliumReady(vm)
	if operatorIsInstalled {
		ExpectCiliumOperatorReady(vm)
	}
	ExpectKubeDNSReady(vm)
}

// SkipIfFlannel will skip the test if it's running over Flannel datapath mode.
func SkipIfFlannel() {
	if helpers.GetCurrentIntegration() == helpers.CIIntegrationFlannel {
		Skip(fmt.Sprintf(
			"This feature is not supported in Cilium %q mode. Skipping test.",
			helpers.CIIntegrationFlannel))
	}
}
