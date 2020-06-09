// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package keystore

import (
	"bytes"
	"text/template"

	"github.com/elastic/cloud-on-k8s/pkg/controller/common/volume"
	corev1 "k8s.io/api/core/v1"
)

const (
	InitContainerName = "elastic-internal-init-keystore"
)

// InitContainerParameters helps to create a valid keystore init script for Kibana or the APM server.
type InitContainerParameters struct {
	// Where the user provided secured settings should be mounted
	SecureSettingsVolumeMountPath string
	// Where the data will be copied
	DataVolumePath string
	// Keystore add command
	KeystoreAddCommand string
	// Keystore create command
	KeystoreCreateCommand string
	// Resources for the init container
	Resources corev1.ResourceRequirements
}

// script is a small bash script to create a Kibana or APM keystore,
// then add all entries from the secure settings secret volume into it.
const script = `#!/usr/bin/env bash

set -eux

echo "Initializing keystore."

# create a keystore in the default data path
{{ .KeystoreCreateCommand }}

# add all existing secret entries into it
for filename in  {{ .SecureSettingsVolumeMountPath }}/*; do
	[[ -e "$filename" ]] || continue # glob does not match
	key=$(basename "$filename")
	echo "Adding "$key" to the keystore."
	{{ .KeystoreAddCommand }}
done

echo "Keystore initialization successful."
`

var scriptTemplate = template.Must(template.New("").Parse(script))

// initContainer returns an init container that executes a bash script
// to load secure settings in a Keystore.
func initContainer(
	secureSettingsSecret volume.SecretVolume,
	volumePrefix string,
	parameters InitContainerParameters,
) (corev1.Container, error) {
	privileged := false
	tplBuffer := bytes.Buffer{}

	if err := scriptTemplate.Execute(&tplBuffer, parameters); err != nil {
		return corev1.Container{}, err
	}

	volumeMounts := []corev1.VolumeMount{
		// access secure settings
		secureSettingsSecret.VolumeMount(),
	}

	// caller might be already taking care of the right mount and volume
	if parameters.DataVolumePath != "" {
		// volume mount to write the keystore in the data volume
		volumeMounts = append(volumeMounts, DataVolume(volumePrefix, parameters.DataVolumePath).VolumeMount())
	}

	return corev1.Container{
		// Image will be inherited from pod template defaults Kibana Docker image
		ImagePullPolicy: corev1.PullIfNotPresent,
		Name:            InitContainerName,
		SecurityContext: &corev1.SecurityContext{
			Privileged: &privileged,
		},
		Command:      []string{"/usr/bin/env", "bash", "-c", tplBuffer.String()},
		VolumeMounts: volumeMounts,
		Resources:    parameters.Resources,
	}, nil
}
