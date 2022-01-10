package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"

	docker "github.com/docker/docker/client"
	"github.com/spf13/cobra"
)

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

	dockerfile *string
)

func init() {
	dockerfile = rootCmd.Flags().StringP("dockerfile", "f", "Dockerfile", "Path to your Dockerfile (or - for stdin)")
	rootCmd.MarkFlagFilename("dockerfile")
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
		fmt.Fprintf(os.Stderr, "Resolving digest of %s...\n", i)
		di, err := c.DistributionInspect(cmd.Context(), i, "")
		if err != nil {
			return err
		}
		pinned[i] = string(di.Descriptor.Digest)
	}
	n := rewriteDockerfileWithDigests(b, pinned)
	return ioutil.WriteFile(ifDash(*dockerfile, "/dev/stdout"), n, 0644)
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

func getUsedBaseImages(dockerfile []byte) []string {
	var ret []string
	for _, l := range bytes.Split(dockerfile, []byte{'\n'}) {
		m := fromRe.FindSubmatch(l)
		if m == nil {
			continue
		}
		ret = append(ret, string(m[2]))
	}
	return ret
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
