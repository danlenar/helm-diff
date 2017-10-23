package main

// This file contains functions that where blatantly copied from
// https://github.wdf.sap.corp/kubernetes/helm

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/ghodss/yaml"
	"google.golang.org/grpc"
	"k8s.io/helm/pkg/downloader"
	"k8s.io/helm/pkg/getter"
	"k8s.io/helm/pkg/repo"
	"k8s.io/helm/pkg/strvals"
)

/////////////// Source: cmd/helm/install.go /////////////////////////

type valueFiles []string

func (v *valueFiles) String() string {
	return fmt.Sprint(*v)
}

func (v *valueFiles) Type() string {
	return "valueFiles"
}

func (v *valueFiles) Set(value string) error {
	for _, filePath := range strings.Split(value, ",") {
		*v = append(*v, filePath)
	}
	return nil
}

// locateChartPath looks for a chart directory in known places, and returns either the full path or an error.
//
// This does not ensure that the chart is well-formed; only that the requested filename exists.
//
// Order of resolution:
// - current working directory
// - if path is absolute or begins with '.', error out here
// - chart repos in $HELM_HOME
// - URL
//
// If 'verify' is true, this will attempt to also verify the chart.
func locateChartPath(repoURL, name, version string, verify bool, keyring,
	certFile, keyFile, caFile string) (string, error) {
	name = strings.TrimSpace(name)
	version = strings.TrimSpace(version)
	if fi, err := os.Stat(name); err == nil {
		abs, err := filepath.Abs(name)
		if err != nil {
			return abs, err
		}
		if verify {
			if fi.IsDir() {
				return "", errors.New("cannot verify a directory")
			}
			if _, err := downloader.VerifyChart(abs, keyring); err != nil {
				return "", err
			}
		}
		return abs, nil
	}
	if filepath.IsAbs(name) || strings.HasPrefix(name, ".") {
		return name, fmt.Errorf("path %q not found", name)
	}

	crepo := filepath.Join(settings.Home.Repository(), name)
	if _, err := os.Stat(crepo); err == nil {
		return filepath.Abs(crepo)
	}

	dl := downloader.ChartDownloader{
		HelmHome: settings.Home,
		Out:      os.Stdout,
		Keyring:  keyring,
		Getters:  getter.All(settings),
	}
	if verify {
		dl.Verify = downloader.VerifyAlways
	}
	if repoURL != "" {
		chartURL, err := repo.FindChartInRepoURL(repoURL, name, version,
			certFile, keyFile, caFile, getter.All(settings))
		if err != nil {
			return "", err
		}
		name = chartURL
	}

	if _, err := os.Stat(settings.Home.Archive()); os.IsNotExist(err) {
		os.MkdirAll(settings.Home.Archive(), 0744)
	}

	filename, _, err := dl.DownloadTo(name, version, settings.Home.Archive())
	if err == nil {
		lname, err := filepath.Abs(filename)
		if err != nil {
			return filename, err
		}
		return lname, nil
	} else if settings.Debug {
		return filename, err
	}

	return filename, fmt.Errorf("file %q not found", name)
}

// Merges source and destination map, preferring values from the source map
func mergeValues(dest map[string]interface{}, src map[string]interface{}) map[string]interface{} {
	for k, v := range src {
		// If the key doesn't exist already, then just set the key to that value
		if _, exists := dest[k]; !exists {
			dest[k] = v
			continue
		}
		nextMap, ok := v.(map[string]interface{})
		// If it isn't another map, overwrite the value
		if !ok {
			dest[k] = v
			continue
		}
		// If the key doesn't exist already, then just set the key to that value
		if _, exists := dest[k]; !exists {
			dest[k] = nextMap
			continue
		}
		// Edge case: If the key exists in the destination, but isn't a map
		destMap, isMap := dest[k].(map[string]interface{})
		// If the source map has a map for this key, prefer it
		if !isMap {
			dest[k] = v
			continue
		}
		// If we got to this point, it is a map in both, so merge them
		dest[k] = mergeValues(destMap, nextMap)
	}
	return dest
}

/////////////// Source: cmd/helm/upgrade.go /////////////////////////

func (d *diffCmd) vals() ([]byte, error) {
	base := map[string]interface{}{}

	// User specified a values files via -f/--values
	for _, filePath := range d.valueFiles {
		currentMap := map[string]interface{}{}
		bytes, err := ioutil.ReadFile(filePath)
		if err != nil {
			return []byte{}, err
		}

		if err := yaml.Unmarshal(bytes, &currentMap); err != nil {
			return []byte{}, fmt.Errorf("failed to parse %s: %s", filePath, err)
		}
		// Merge with the previous map
		base = mergeValues(base, currentMap)
	}

	// User specified a value via --set
	for _, value := range d.values {
		if err := strvals.ParseInto(value, base); err != nil {
			return []byte{}, fmt.Errorf("failed parsing --set data: %s", err)
		}
	}

	return yaml.Marshal(base)
}

/////////////// Source: cmd/helm/helm.go ////////////////////////////

func checkArgsLength(argsReceived int, requiredArgs ...string) error {
	expectedNum := len(requiredArgs)
	if argsReceived != expectedNum {
		arg := "arguments"
		if expectedNum == 1 {
			arg = "argument"
		}
		return fmt.Errorf("This command needs %v %s: %s", expectedNum, arg, strings.Join(requiredArgs, ", "))
	}
	return nil
}

func prettyError(err error) error {
	if err == nil {
		return nil
	}
	// This is ridiculous. Why is 'grpc.rpcError' not exported? The least they
	// could do is throw an interface on the lib that would let us get back
	// the desc. Instead, we have to pass ALL errors through this.
	return errors.New(grpc.ErrorDesc(err))
}
