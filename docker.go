package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	docker "github.com/docker/docker/client"
	"github.com/spf13/cobra"
)

type DockerBaseReference struct {
	BaseName     string
	Sha256SumTag string
}

var (
	dockerCmd = &cobra.Command{
		Use:   "docker pin",
		Short: "Pinning docker base images in a Dockerfile",
	}

	dockerPinCmd = &cobra.Command{
		Use:          "pin [-f Dockerfile]",
		Example:      "pin [-f -] < Dockerfile.in > Dockerfile.out",
		Short:        "Update the Dockerfile to pin the current digest of the images",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runDockerPin,
	}

	dockerResolveCmd = &cobra.Command{
		Use:          "resolve [base-image]",
		Example:      "resolve ubuntu:20.04",
		Short:        "Prints the current digest of the given base image",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         runDockerResolve,
	}

	dockerCheckCmd = &cobra.Command{
		Use:          "check [-f Dockerfile]",
		Example:      "check [-f -] < Dockerfile",
		Short:        "Check if any base image in a Dockerfile is not the latest tagged image",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runDockerCheck,
	}

	dockerfile *string
)

func init() {
	dockerfile = rootCmd.PersistentFlags().StringP("dockerfile", "f", "Dockerfile", "Path to your Dockerfile (or - for stdin)")
	rootCmd.MarkPersistentFlagFilename("dockerfile")
}

func runDockerPin(cmd *cobra.Command, args []string) error {
	b, err := ioutil.ReadFile(ifDash(*dockerfile, "/dev/stdin"))
	if err != nil {
		return fmt.Errorf("failed to read %q: %v", *dockerfile, err)
	}
	c, err := docker.NewClientWithOpts(docker.FromEnv)
	if err != nil {
		return err
	}
	pinned := map[string]string{}
	images := getUsedBaseImages(b)
	for _, i := range images {
		fmt.Fprintf(os.Stderr, "Resolving digest of %s...\n", i.BaseName)
		di, err := c.DistributionInspect(cmd.Context(), i.BaseName, "")
		if err != nil {
			return err
		}
		pinned[i.BaseName] = string(di.Descriptor.Digest)
	}
	n := rewriteDockerfileWithDigests(b, pinned)
	return ioutil.WriteFile(ifDash(*dockerfile, "/dev/stdout"), n, 0644)
}

func runDockerResolve(cmd *cobra.Command, args []string) error {
	baseImageAndVersion := args[0]
	dockerClient, err := docker.NewClientWithOpts(docker.FromEnv)
	if err != nil {
		return err
	}
	di, err := dockerClient.DistributionInspect(cmd.Context(), baseImageAndVersion, "")
	if err != nil {
		return err
	}
	hashedBase := baseImageAndVersion + "@" + string(di.Descriptor.Digest)
	_, err = fmt.Println(hashedBase)
	return err
}

func runDockerCheck(cmd *cobra.Command, args []string) error {
	b, err := ioutil.ReadFile(ifDash(*dockerfile, "/dev/stdin"))
	if err != nil {
		return fmt.Errorf("failed to read %q: %v", *dockerfile, err)
	}
	c, err := docker.NewClientWithOpts(docker.FromEnv)
	if err != nil {
		return err
	}
	pinned := map[string]string{}
	images := getUsedBaseImages(b)
	allUpToDate := true
	for _, i := range images {
		fmt.Fprintf(os.Stderr, "Resolving digest of %s...\n", i.BaseName)
		di, err := c.DistributionInspect(cmd.Context(), i.BaseName, "")
		if err != nil {
			return err
		}
		if len(i.Sha256SumTag) == 0 || i.Sha256SumTag != string(di.Descriptor.Digest) {
			fmt.Fprintf(os.Stderr, "%v is not at its latest!\n", i.BaseName)
			allUpToDate = false
		}
		pinned[i.BaseName] = string(di.Descriptor.Digest)
	}
	if allUpToDate == false {
		return fmt.Errorf("Some image(s) are not tagged at latest")
	}
	return nil
}

func ifDash(fn string, repl string) string {
	if fn == "-" {
		return repl
	}
	return fn
}

var (
	fromRe = regexp.MustCompile(`^(FROM\s+(?:--\S+\s+)*([^@ ]+))(@\S+)?(.*)$`)
)

func getUsedBaseImages(dockerfile []byte) []DockerBaseReference {
	var ret []DockerBaseReference
	for _, l := range bytes.Split(dockerfile, []byte{'\n'}) {
		m := fromRe.FindSubmatch(l)
		if m == nil {
			continue
		}
		if string(m[2]) == "scratch" {
			continue
		}
		var base DockerBaseReference
		base.BaseName = string(m[2])
		if len(m) > 2 {
			base.Sha256SumTag = strings.Replace(string(m[3]), "@", "", 1)
		}
		ret = append(ret, base)
	}
	return ret
}

func getLastBaseImage(dockerfile []byte) string {
	var last string
	for _, l := range bytes.Split(dockerfile, []byte{'\n'}) {
		m := fromRe.FindSubmatch(l)
		if m != nil {
			if string(m[2]) == "scratch" {
				continue
			}
			last = string(m[2]) + string(m[3])
		}
	}
	return last
}

func rewriteDockerfileWithDigests(dockerfile []byte, images map[string]string) []byte {
	var buf bytes.Buffer
	for i, l := range bytes.Split(dockerfile, []byte{'\n'}) {
		if i > 0 {
			buf.WriteByte('\n')
		}
		m := fromRe.FindSubmatch(l)
		if m != nil {
			if d, ok := images[string(m[2])]; ok {
				buf.Write(m[1])
				buf.WriteByte('@')
				buf.WriteString(d)
				buf.Write(m[4])
				continue
			}
		}
		buf.Write(l)
	}
	return buf.Bytes()
}
