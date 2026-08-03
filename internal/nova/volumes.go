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

package nova

import (
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
)

const (
	scriptVolume = "scripts"
	configVolume = "config-data"
	logVolume    = "logs"
	// RunHttpdVolume is the volume name used for the httpd PID file directory
	RunHttpdVolume = "run-httpd"
	// VarLogHttpdVolume is the volume name used for httpd's own logs
	VarLogHttpdVolume = "var-log-httpd"
)

var (
	configMode int32 = 0640
	scriptMode int32 = 0740
)

// GetConfVolumeMounts returns the final-path SubPath mounts for the config
// snippets shared by every Nova service: nova.conf, nova.conf.d/01-nova.conf,
// nova.conf.d/02-nova-override.conf (only when the service has a
// CustomServiceConfig set, since that key is only added to the config Secret
// in that case), and /etc/my.cnf.
func GetConfVolumeMounts(hasCustomServiceConfig bool) []corev1.VolumeMount {
	vm := []corev1.VolumeMount{
		{
			Name:      configVolume,
			MountPath: "/etc/nova/nova.conf",
			SubPath:   "nova-blank.conf",
			ReadOnly:  true,
		},
		{
			Name:      configVolume,
			MountPath: "/etc/nova/nova.conf.d/01-nova.conf",
			SubPath:   "01-nova.conf",
			ReadOnly:  true,
		},
	}
	if hasCustomServiceConfig {
		vm = append(vm, corev1.VolumeMount{
			Name:      configVolume,
			MountPath: "/etc/nova/nova.conf.d/02-nova-override.conf",
			SubPath:   "02-nova-override.conf",
			ReadOnly:  true,
		})
	}
	vm = append(vm, corev1.VolumeMount{
		Name:      configVolume,
		MountPath: "/etc/my.cnf",
		SubPath:   "my.cnf",
		ReadOnly:  true,
	})
	return vm
}

// GetConfigOverwriteVolumeMounts returns SubPath volume mounts that place
// each defaultConfigOverwrite key as an individual file under basePath
// (e.g. /etc/nova/policy.yaml, /etc/nova/api-paste.ini). The overwrite data
// lives in the same config Secret (merged in via CustomData).
func GetConfigOverwriteVolumeMounts(overwriteKeys []string, basePath string) []corev1.VolumeMount {
	mounts := make([]corev1.VolumeMount, 0, len(overwriteKeys))
	sorted := make([]string, len(overwriteKeys))
	copy(sorted, overwriteKeys)
	slices.Sort(sorted)
	for _, key := range sorted {
		mounts = append(mounts, corev1.VolumeMount{
			Name:      configVolume,
			MountPath: fmt.Sprintf("%s/%s", basePath, key),
			SubPath:   key,
			ReadOnly:  true,
		})
	}
	return mounts
}

// GetRunHttpdVolume returns the emptyDir Volume used for the httpd PID file
func GetRunHttpdVolume() corev1.Volume {
	return corev1.Volume{
		Name: RunHttpdVolume,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVarLogHttpdVolume returns the emptyDir Volume used for httpd's own logs
func GetVarLogHttpdVolume() corev1.Volume {
	return corev1.Volume{
		Name: VarLogHttpdVolume,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetConfigVolume returns a volume for Nova configuration files from a secret
func GetConfigVolume(secretName string) corev1.Volume {
	return corev1.Volume{
		Name: configVolume,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				DefaultMode: &configMode,
				SecretName:  secretName,
			},
		},
	}
}

// GetLogVolumeMount returns a volume mount for Nova log files
func GetLogVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      logVolume,
		MountPath: "/var/log/nova",
		ReadOnly:  false,
	}
}

// GetLogVolume returns an empty directory volume for Nova log files
func GetLogVolume() corev1.Volume {
	return corev1.Volume{
		Name: logVolume,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{Medium: ""},
		},
	}
}

// GetScriptVolumeMount returns a volume mount for Nova script files
func GetScriptVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      scriptVolume,
		MountPath: "/var/lib/openstack/bin",
		ReadOnly:  false,
	}
}

// GetScriptVolume returns a volume for Nova script files from a secret
func GetScriptVolume(secretName string) corev1.Volume {
	return corev1.Volume{
		Name: scriptVolume,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				DefaultMode: &scriptMode,
				SecretName:  secretName,
			},
		},
	}
}
