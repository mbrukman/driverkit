package builder

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/falcosecurity/driverkit/pkg/kernelrelease"
)

// TargetTypeUbuntuGeneric identifies the UbuntuGeneric target.
const TargetTypeUbuntuGeneric Type = "ubuntu-generic"

// TargetTypeUbuntuAWS identifies the UbuntuAWS target.
const TargetTypeUbuntuAWS Type = "ubuntu-aws"

func init() {
	BuilderByTarget[TargetTypeUbuntuGeneric] = &ubuntuGeneric{}
	BuilderByTarget[TargetTypeUbuntuAWS] = &ubuntuAWS{}
}

// ubuntuGeneric is a driverkit target.
type ubuntuGeneric struct {
}

// Script compiles the script to build the kernel module and/or the eBPF probe.
func (v ubuntuGeneric) Script(c Config) (string, error) {
	t := template.New(string(TargetTypeUbuntuGeneric))
	parsed, err := t.Parse(ubuntuTemplate)
	if err != nil {
		return "", err
	}

	kr := kernelrelease.FromString(c.Build.KernelRelease)

	urls, err := getResolvingURLs(fetchUbuntuGenericKernelURL(kr, c.Build.KernelVersion))
	if err != nil {
		return "", err
	}
	if len(urls) != 2 {
		return "", fmt.Errorf("specific kernel headers not found")
	}

	td := ubuntuTemplateData{
		DriverBuildDir:       DriverDirectory,
		ModuleDownloadURL:    fmt.Sprintf("%s/%s.tar.gz", c.DownloadBaseURL, c.Build.DriverVersion),
		KernelDownloadURLS:   urls,
		KernelLocalVersion:   kr.FullExtraversion,
		KernelHeadersPattern: "*generic",
		BuildModule:          len(c.Build.ModuleFilePath) > 0,
		BuildProbe:           len(c.Build.ProbeFilePath) > 0,
	}

	buf := bytes.NewBuffer(nil)
	err = parsed.Execute(buf, td)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// ubuntuAWS is a driverkit target.
type ubuntuAWS struct {
}

// Script compiles the script to build the kernel module and/or the eBPF probe.
func (v ubuntuAWS) Script(c Config) (string, error) {
	t := template.New(string(TargetTypeUbuntuGeneric))
	parsed, err := t.Parse(ubuntuTemplate)
	if err != nil {
		return "", err
	}

	kr := kernelrelease.FromString(c.Build.KernelRelease)

	urls, err := getResolvingURLs(fetchUbuntuAWSKernelURLS(kr, c.Build.KernelVersion))
	if err != nil {
		return "", err
	}
	if len(urls) != 2 {
		return "", fmt.Errorf("specific kernel headers not found")
	}

	td := ubuntuTemplateData{
		DriverBuildDir:       DriverDirectory,
		ModuleDownloadURL:    moduleDownloadURL(c),
		KernelDownloadURLS:   urls,
		KernelLocalVersion:   kr.FullExtraversion,
		KernelHeadersPattern: "*",
		BuildModule:          len(c.Build.ModuleFilePath) > 0,
		BuildProbe:           len(c.Build.ProbeFilePath) > 0,
	}

	buf := bytes.NewBuffer(nil)
	err = parsed.Execute(buf, td)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func fetchUbuntuGenericKernelURL(kr kernelrelease.KernelRelease, kernelVersion uint16) []string {
	firstExtra := extractExtraNumber(kr.Extraversion)
	return []string{
		fmt.Sprintf(
			"https://mirrors.edge.kernel.org/ubuntu/pool/main/l/linux/linux-headers-%s-%s_%s-%s.%d_all.deb",
			kr.Fullversion,
			firstExtra,
			kr.Fullversion,
			firstExtra,
			kernelVersion,
		),
		fmt.Sprintf(
			"https://mirrors.edge.kernel.org/ubuntu/pool/main/l/linux/linux-headers-%s%s_%s-%s.%d_amd64.deb",
			kr.Fullversion,
			kr.FullExtraversion,
			kr.Fullversion,
			firstExtra,
			kernelVersion,
		),
	}
}

func fetchUbuntuAWSKernelURLS(kr kernelrelease.KernelRelease, kernelVersion uint16) []string {
	firstExtra := extractExtraNumber(kr.Extraversion)
	return []string{
		fmt.Sprintf(
			"https://mirrors.edge.kernel.org/ubuntu/pool/main/l/linux-aws/linux-aws-headers-%s-%s_%s-%s.%d_all.deb",
			kr.Fullversion,
			firstExtra,
			kr.Fullversion,
			firstExtra,
			kernelVersion,
		),
		fmt.Sprintf(
			"https://mirrors.edge.kernel.org/ubuntu/pool/main/l/linux-aws/linux-headers-%s%s_%s-%s.%d_amd64.deb",
			kr.Fullversion,
			kr.FullExtraversion,
			kr.Fullversion,
			firstExtra,
			kernelVersion,
		),
	}
}

func extractExtraNumber(extraversion string) string {
	firstExtraSplit := strings.Split(extraversion, "-")
	if len(firstExtraSplit) > 0 {
		return firstExtraSplit[0]
	}
	return ""
}

type ubuntuTemplateData struct {
	DriverBuildDir       string
	ModuleDownloadURL    string
	KernelDownloadURLS   []string
	KernelLocalVersion   string
	KernelHeadersPattern string
	BuildProbe           bool
	BuildModule          bool
}

const ubuntuTemplate = `
#!/bin/bash
set -xeuo pipefail

rm -Rf {{ .DriverBuildDir }}
mkdir {{ .DriverBuildDir }}
rm -Rf /tmp/module-download
mkdir -p /tmp/module-download

curl --silent -SL {{ .ModuleDownloadURL }} | tar -xzf - -C /tmp/module-download
mv /tmp/module-download/*/driver/* {{ .DriverBuildDir }}

cp /driverkit/module-Makefile {{ .DriverBuildDir }}/Makefile
cp /driverkit/module-driver-config.h {{ .DriverBuildDir }}/driver_config.h

# Fetch the kernel
mkdir /tmp/kernel-download
cd /tmp/kernel-download
{{range $url := .KernelDownloadURLS}}
curl --silent -o kernel.deb -SL {{ $url }}
ar x kernel.deb
tar -xvf data.tar.xz
{{end}}
ls -la /tmp/kernel-download

cd /tmp/kernel-download/usr/src/
sourcedir=$(find . -type d -name "linux-headers{{ .KernelHeadersPattern }}" | head -n 1 | xargs readlink -f)

ls -la $sourcedir

{{ if .BuildModule }}
# Build the module
cd {{ .DriverBuildDir }}
make KERNELDIR=$sourcedir
strip -g falco.ko
# Print results
modinfo falco.ko
{{ end }}

{{ if .BuildProbe }}
# Build the eBPF probe
cd {{ .DriverBuildDir }}/bpf
make LLC=/usr/bin/llc-7 CLANG=/usr/bin/clang-7 CC=/usr/bin/gcc-8 KERNELDIR=$sourcedir
ls -l probe.o
{{ end }}
`
