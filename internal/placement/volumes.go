/*

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

package placement

import (
	"github.com/openstack-k8s-operators/lib-common/modules/common/pod"
	internalcommon "github.com/openstack-k8s-operators/nova-operator/internal/common"

	corev1 "k8s.io/api/core/v1"
)

// placementHomeVolume is the writable emptyDir for subdirectories of
// /var/lib/placement (the placement user's home dir) needed under
// ReadOnlyRootFilesystem -- see pod.WritableHomeDirMounts.
const placementHomeVolume = "var-lib-placement"

// getVolumes - service volumes
func getVolumes(name string) []corev1.Volume {
	var scriptsVolumeDefaultMode int32 = 0755
	var configMode int32 = 0640

	return []corev1.Volume{
		{
			Name: "scripts",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &scriptsVolumeDefaultMode,
					SecretName:  internalcommon.GetScriptSecretName(name),
				},
			},
		},
		{
			Name: "config-data",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					DefaultMode: &configMode,
					SecretName:  internalcommon.GetServiceConfigSecretName(name),
				},
			},
		},
		{
			Name: "logs",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{Medium: ""},
			},
		},
		{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		// Writable /var/lib/placement/.cache: RHEL's python3-setuptools/
		// pkg_resources downstream patch caches iter_entry_points() scans
		// under $HOME/.cache/python-entrypoints/<hash> on every process
		// start (confirmed via real-cluster inspection across multiple
		// operators in the remove-kolla effort). placement.conf sets no
		// lock_path/state_path, so only .cache is needed here, not a
		// home-dir "tmp" (any oslo_concurrency locking would fall back to
		// the already-mounted system /tmp above).
		pod.WritableHomeDirVolume(placementHomeVolume, nil),
	}

}

// getVolumeMounts - API deployment VolumeMounts
func getVolumeMounts(withPolicy bool) []corev1.VolumeMount {
	vm := []corev1.VolumeMount{
		{
			Name:      "scripts",
			MountPath: "/usr/local/bin/container-scripts",
			ReadOnly:  true,
		},
		{
			Name:      "logs",
			MountPath: "/var/log/placement",
			ReadOnly:  false,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/placement/placement.conf",
			SubPath:   "placement.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/placement/placement.conf.d/custom.conf",
			SubPath:   "custom.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/httpd/conf/httpd.conf",
			SubPath:   "httpd.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/httpd/conf.d/ssl.conf",
			SubPath:   "ssl.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
		{
			Name:      "run-httpd",
			MountPath: "/run/httpd",
		},
		{
			Name:      "tmp",
			MountPath: "/tmp",
		},
		{
			Name:      "var-log-httpd",
			MountPath: "/var/log/httpd",
		},
	}
	vm = append(vm, pod.WritableHomeDirMounts(placementHomeVolume, "/var/lib/placement", ".cache")...)
	if withPolicy {
		vm = append(vm, corev1.VolumeMount{
			Name:      "config-data",
			MountPath: "/etc/placement/policy.yaml",
			SubPath:   "policy.yaml",
			ReadOnly:  true,
		})
	}
	return vm
}

// getDBSyncVolumeMounts - db-sync job VolumeMounts
func getDBSyncVolumeMounts(withPolicy bool) []corev1.VolumeMount {
	vm := []corev1.VolumeMount{
		{
			Name:      "scripts",
			MountPath: "/usr/local/bin/container-scripts",
			ReadOnly:  true,
		},
		{
			Name:      "logs",
			MountPath: "/var/log/placement",
			ReadOnly:  false,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/placement/placement.conf",
			SubPath:   "placement.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/placement/placement.conf.d/custom.conf",
			SubPath:   "custom.conf",
			ReadOnly:  true,
		},
		{
			Name:      "config-data",
			MountPath: "/etc/my.cnf",
			SubPath:   "my.cnf",
			ReadOnly:  true,
		},
		{
			Name:      "tmp",
			MountPath: "/tmp",
		},
	}
	vm = append(vm, pod.WritableHomeDirMounts(placementHomeVolume, "/var/lib/placement", ".cache")...)
	if withPolicy {
		vm = append(vm, corev1.VolumeMount{
			Name:      "config-data",
			MountPath: "/etc/placement/policy.yaml",
			SubPath:   "policy.yaml",
			ReadOnly:  true,
		})
	}
	return vm
}
