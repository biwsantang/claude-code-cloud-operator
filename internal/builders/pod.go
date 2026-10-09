// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package builders

import (
	"fmt"
	api "github.com/biwsantang/claude-code-cloud-operator/api/v1alpha1"
	"github.com/biwsantang/claude-code-cloud-operator/internal/contract"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

func Labels(fleet, revision, role string) map[string]string {
	return map[string]string{contract.FleetLabel: fleet, contract.RevisionLabel: revision[:32], contract.RoleLabel: role}
}
func Security() *corev1.SecurityContext {
	return &corev1.SecurityContext{RunAsNonRoot: ptr.To(true), RunAsUser: ptr.To(int64(1000)), RunAsGroup: ptr.To(int64(1000)), ReadOnlyRootFilesystem: ptr.To(true), AllowPrivilegeEscalation: ptr.To(false), Capabilities: &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}}, SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}}
}
func Ephemeral(name, size string) corev1.Volume {
	return corev1.Volume{Name: name, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: ptr.To(resource.MustParse(size))}}}
}
func ProxyEnv(p api.ProxyPolicy) []corev1.EnvVar {
	return []corev1.EnvVar{{Name: "HTTPS_PROXY", Value: p.URL}, {Name: "HTTP_PROXY", Value: p.URL}, {Name: "https_proxy", Value: p.URL}, {Name: "http_proxy", Value: p.URL}, {Name: "NO_PROXY", Value: "127.0.0.1,localhost"}, {Name: "DISABLE_AUTOUPDATER", Value: "1"}, {Name: "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", Value: "1"}, {Name: "HOME", Value: "/home/runner"}, {Name: "TMPDIR", Value: "/tmp"}}
}
func Runner(w *api.ClaudeWorkOrder) *corev1.Pod {
	p := w.Spec.Execution
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: w.Name, Namespace: w.Namespace, Labels: Labels(w.Spec.FleetName, contract.NetworkDigest(p), "session"), OwnerReferences: []metav1.OwnerReference{contract.Owner(w, "ClaudeWorkOrder")}}, Spec: corev1.PodSpec{RestartPolicy: corev1.RestartPolicyNever, AutomountServiceAccountToken: ptr.To(false), ServiceAccountName: w.Spec.FleetName + "-session", EnableServiceLinks: ptr.To(false), TerminationGracePeriodSeconds: ptr.To(int64(120)), SecurityContext: &corev1.PodSecurityContext{FSGroup: ptr.To(int64(1000)), RunAsNonRoot: ptr.To(true), SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}}, NodeSelector: p.NodeSelector, Tolerations: p.Tolerations, Volumes: []corev1.Volume{Ephemeral("workspace", p.WorkspaceSize), Ephemeral("home", "1Gi"), Ephemeral("tmp", "1Gi"), {Name: "credential", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: w.Spec.CredentialSecretRef.Name, DefaultMode: ptr.To(int32(0440)), Items: []corev1.KeyToPath{{Key: contract.CredentialKey, Path: "work-order.jwt"}}}}}}, Containers: []corev1.Container{{Name: "runner", Image: p.RunnerImage, Command: []string{"/usr/local/bin/claude"}, Args: []string{"self-hosted-runner", "--environment-secret-file", "/credential/work-order.jwt", "--base-dir", "/workspace", "--capacity", "1", "--drain-grace-sec", "0", "--confine-repo-settings", "enforce", "--use-anthropic-git-proxy", "--configure-git"}, SecurityContext: Security(), Resources: p.Resources, Env: ProxyEnv(p.Proxy), VolumeMounts: []corev1.VolumeMount{{Name: "credential", MountPath: "/credential", ReadOnly: true}, {Name: "workspace", MountPath: "/workspace"}, {Name: "home", MountPath: "/home/runner"}, {Name: "tmp", MountPath: "/tmp"}}}}}}
	if p.Lifecycle != nil {
		life := contract.ResolvedLifecycle(p.Lifecycle)
		pod.Spec.TerminationGracePeriodSeconds = ptr.To(int64(*life.ShutdownWaitSeconds) + 120)
		pod.Spec.Containers[0].Args = append(pod.Spec.Containers[0].Args,
			"--release-idle-session-min", fmt.Sprint(*life.IdleMinutes),
			"--kill-session-after-min", fmt.Sprint(*life.MaxSessionMinutes),
			"--drain-wait-sec", fmt.Sprint(*life.ShutdownWaitSeconds),
			"--session-stop-grace-sec", "5", "--post-session-hook-timeout-sec", "60")
		if *life.PushOutcomeOnRelease {
			// Native 2.1.285 can add 20s when release was already in flight.
			// 80s fixed + 30s push + 20s release + 10s headroom = 140s.
			pod.Spec.TerminationGracePeriodSeconds = ptr.To(int64(*life.ShutdownWaitSeconds) + 140)
			pod.Spec.Containers[0].Args = append(pod.Spec.Containers[0].Args, "--push-outcome-on-release")
		}
		// Materialize image config even with the reminder disabled: the session's
		// empty HOME must not hide image-provided hooks, permissions or MCP definitions.
		{
			pod.Spec.Volumes = append(pod.Spec.Volumes, Ephemeral("session-tools", "64Mi"), Ephemeral("host-config", "64Mi"))
			resources := corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")}, Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("256Mi")}}
			pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{Name: "install-session-tools", Image: p.SessionConfigImage, Command: []string{"/spawn-runner", "install", "/session-tools"}, SecurityContext: Security(), Resources: resources, VolumeMounts: []corev1.VolumeMount{{Name: "session-tools", MountPath: "/session-tools"}}})
			prepare := corev1.Container{Name: "prepare-host-config", Image: p.RunnerImage, Command: []string{"/session-tools/spawn-runner", "prepare-session-config", "/host-config", fmt.Sprint(*life.PromptToSave)}, SecurityContext: Security(), Resources: resources, VolumeMounts: []corev1.VolumeMount{{Name: "session-tools", MountPath: "/session-tools", ReadOnly: true}, {Name: "host-config", MountPath: "/host-config"}}}
			if p.HostConfigRef != nil {
				pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{Name: "host-config-source", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: p.HostConfigRef.Name}}}})
				prepare.Command = append(prepare.Command, "/host-config-source")
				prepare.VolumeMounts = append(prepare.VolumeMounts, corev1.VolumeMount{Name: "host-config-source", MountPath: "/host-config-source", ReadOnly: true})
			}
			pod.Spec.InitContainers = append(pod.Spec.InitContainers, prepare)
			pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{Name: "host-config", MountPath: "/etc/claude", ReadOnly: true})
			pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, corev1.EnvVar{Name: "SELF_HOSTED_RUNNER_HOST_CONFIG_DIR", Value: "/etc/claude"})
		}
	} else if p.HostConfigRef != nil {
		// Native config snapshots omit symlinks. Materialize projected ConfigMap files before startup.
		pod.Spec.Volumes = append(pod.Spec.Volumes, Ephemeral("host-config", "32Mi"), corev1.Volume{Name: "host-config-source", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: p.HostConfigRef.Name}}}})
		pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{
			Name: "prepare-host-config", Image: p.RunnerImage, ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{"/bin/sh", "-c", "set -eu; for file in /host-config-source/* /host-config-source/.[!.]* /host-config-source/..?*; do [ -f \"$file\" ] || continue; cp -- \"$file\" /host-config/; done"},
			SecurityContext: Security(), Resources: p.Resources,
			TerminationMessagePath: corev1.TerminationMessagePathDefault, TerminationMessagePolicy: corev1.TerminationMessageReadFile,
			VolumeMounts: []corev1.VolumeMount{{Name: "host-config-source", MountPath: "/host-config-source", ReadOnly: true}, {Name: "host-config", MountPath: "/host-config"}},
		})
		pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{Name: "host-config", MountPath: "/etc/claude", ReadOnly: true})
		pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, corev1.EnvVar{Name: "SELF_HOSTED_RUNNER_HOST_CONFIG_DIR", Value: "/etc/claude"})
	}
	return pod
}
func Network(f *api.ClaudeRunnerFleet) *networkingv1.NetworkPolicy {
	revision := contract.NetworkDigest(f.Spec.Execution)
	p := f.Spec.Execution.Proxy
	return &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: f.Name + "-" + revision[:12], Namespace: f.Namespace, Annotations: map[string]string{contract.Group + "/declared-policy-digest": contract.PolicyDigest(f.Spec.Execution)}, OwnerReferences: []metav1.OwnerReference{contract.Owner(f, "ClaudeRunnerFleet")}}, Spec: networkingv1.NetworkPolicySpec{PodSelector: metav1.LabelSelector{MatchLabels: Labels(f.Name, revision, "session")}, PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress, networkingv1.PolicyTypeEgress}, Ingress: []networkingv1.NetworkPolicyIngressRule{}, Egress: []networkingv1.NetworkPolicyEgressRule{{To: []networkingv1.NetworkPolicyPeer{{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": p.Namespace}}, PodSelector: &metav1.LabelSelector{MatchLabels: p.PodLabels}}}, Ports: []networkingv1.NetworkPolicyPort{{Protocol: ptr.To(corev1.ProtocolTCP), Port: ptr.To(intstr.FromInt32(p.Port))}}}, {To: []networkingv1.NetworkPolicyPeer{{NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"kubernetes.io/metadata.name": "kube-system"}}, PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"k8s-app": "kube-dns"}}}}, Ports: []networkingv1.NetworkPolicyPort{{Protocol: ptr.To(corev1.ProtocolUDP), Port: ptr.To(intstr.FromInt32(53))}, {Protocol: ptr.To(corev1.ProtocolTCP), Port: ptr.To(intstr.FromInt32(53))}}}}}}
}
func Orchestrator(f *api.ClaudeRunnerFleet, credentialRevision string, enabled bool) *appsv1.Deployment {
	replicas := int32(0)
	if enabled {
		replicas = f.Spec.OrchestratorReplicas
	}
	labels := map[string]string{contract.FleetLabel: f.Name, contract.RoleLabel: "orchestrator"}
	mounts := []corev1.VolumeMount{{Name: "credential", MountPath: "/credential", ReadOnly: true}, {Name: "hook", MountPath: "/operator-hooks", ReadOnly: true}, {Name: "home", MountPath: "/home/runner"}, {Name: "tmp", MountPath: "/tmp"}}
	resources := corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("256Mi")}, Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("512Mi")}}
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: f.Name + "-orchestrator", Namespace: f.Namespace, OwnerReferences: []metav1.OwnerReference{contract.Owner(f, "ClaudeRunnerFleet")}}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}, Selector: &metav1.LabelSelector{MatchLabels: labels}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels, Annotations: map[string]string{contract.Group + "/credential-revision": credentialRevision}}, Spec: corev1.PodSpec{DNSPolicy: corev1.DNSClusterFirst, RestartPolicy: corev1.RestartPolicyAlways, SchedulerName: corev1.DefaultSchedulerName, TerminationGracePeriodSeconds: ptr.To(int64(30)), ServiceAccountName: contract.HookAccount(f.Name), AutomountServiceAccountToken: ptr.To(true), EnableServiceLinks: ptr.To(false), NodeSelector: f.Spec.Execution.NodeSelector, Tolerations: f.Spec.Execution.Tolerations, SecurityContext: &corev1.PodSecurityContext{FSGroup: ptr.To(int64(1000)), RunAsNonRoot: ptr.To(true), SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}}, Volumes: []corev1.Volume{Ephemeral("hook", "32Mi"), Ephemeral("home", "128Mi"), Ephemeral("tmp", "128Mi"), {Name: "credential", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: f.Spec.EnvironmentSecretRef.Name, DefaultMode: ptr.To(int32(0440)), Items: []corev1.KeyToPath{{Key: contract.EnvironmentKey, Path: "environment-secret"}}}}}}, InitContainers: []corev1.Container{{Name: "install-hook", ImagePullPolicy: corev1.PullIfNotPresent, TerminationMessagePath: corev1.TerminationMessagePathDefault, TerminationMessagePolicy: corev1.TerminationMessageReadFile, Image: f.Spec.HookImage, Command: []string{"/spawn-runner", "install", "/operator-hooks"}, SecurityContext: Security(), Resources: resources, VolumeMounts: []corev1.VolumeMount{{Name: "hook", MountPath: "/operator-hooks"}}}}, Containers: []corev1.Container{{Name: "orchestrator", ImagePullPolicy: corev1.PullIfNotPresent, TerminationMessagePath: corev1.TerminationMessagePathDefault, TerminationMessagePolicy: corev1.TerminationMessageReadFile, Image: f.Spec.OrchestratorImage, Command: []string{"/usr/local/bin/claude"}, Args: []string{"self-hosted-runner", "orchestrator", "--environment-secret-file", "/credential/environment-secret", "--hooks-dir", "/operator-hooks", "--hook-timeout", fmt.Sprint(f.Spec.HookTimeoutSeconds), "--expected-spawn-seconds", fmt.Sprint(f.Spec.SpawnLeaseSeconds), "--min-idle", "0", "--health-port", "8080"}, Env: []corev1.EnvVar{{Name: "OPERATOR_NAMESPACE", Value: f.Namespace}, {Name: "OPERATOR_FLEET", Value: f.Name}, {Name: "HOME", Value: "/home/runner"}, {Name: "DISABLE_AUTOUPDATER", Value: "1"}}, SecurityContext: Security(), Resources: resources, VolumeMounts: mounts, ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"/operator-hooks/spawn-runner", "probe"}}}, SuccessThreshold: 1, FailureThreshold: 3, PeriodSeconds: 5, TimeoutSeconds: 3}, LivenessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Scheme: corev1.URISchemeHTTP, Path: "/healthz", Port: intstr.FromInt32(8080)}}, TimeoutSeconds: 1, SuccessThreshold: 1, FailureThreshold: 3, InitialDelaySeconds: 10, PeriodSeconds: 10}}}}}}}
}
