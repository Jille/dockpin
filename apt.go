package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/alessio/shellescape"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
)

var (
	aptCmd = &cobra.Command{
		Use:   "apt {pin|install}",
		Short: "Pinning and installing apt packages",
	}

	aptPinCmd = &cobra.Command{
		Use:          "pin",
		Short:        "Pin which versions to install (but don't install them)",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runAptPin,
	}

	aptInstallCmd = &cobra.Command{
		Use:          "install",
		Short:        "Install the pinned versions",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runAptInstall,
	}

	aptPinFile          *string
	aptRequirementsFile *string
	baseImage           *string
)

func init() {
	aptPinFile = aptCmd.PersistentFlags().StringP("pin-file", "p", "dockpin-apt.lock", "File with pinned package versions")
	aptCmd.MarkPersistentFlagFilename("pin-file", "lock")
	aptRequirementsFile = aptPinCmd.Flags().StringP("selection-file", "s", "dockpin-apt.pkgs", "File with packages to be installed")
	aptPinCmd.MarkPersistentFlagFilename("selection-file", "pkgs")
	baseImage = aptPinCmd.Flags().String("base-image", "", "Docker image you're going to use dockpin in, so we can figure out your additional dependencies.")
}

func runAptPin(cmd *cobra.Command, args []string) error {
	b, err := ioutil.ReadFile(*aptRequirementsFile)
	if err != nil {
		return fmt.Errorf("failed to read selection file %q: %v", *aptRequirementsFile, err)
	}

	if *baseImage == "" {
		b, err := ioutil.ReadFile(ifDash(*dockerfile, "/dev/stdin"))
		if err != nil {
			return fmt.Errorf("failed to read %q (needed to determine your base image): %v", *dockerfile, err)
		}
		*baseImage = getLastBaseImage(b)
		if *baseImage == "" {
			return errors.New("no images found in your Dockerfile")
		}
		fmt.Fprintf(os.Stderr, "Based on your Dockerfile, it looks like you'll use dockpin in an image based on %s. Pass --base-image if that's incorrect.\n", *baseImage)
	}

	// Let me know if you know a nice way that doesn't depend on composing a shell script.
	shcmd := "apt-get update >&2 && echo Determining dependencies... >&2 && apt-get install --print-uris -qq --no-install-recommends"
	for _, p := range strings.Split(string(b), "\n") {
		shcmd += " " + shellescape.Quote(p)
	}
	var buf bytes.Buffer
	buf.WriteString("# dockpin apt lock file v1\n")
	buf.WriteString("base-image=" + *baseImage + "\n")
	buf.WriteString("\n")
	c := exec.Command("docker", "run", "--rm", *baseImage, "bash", "-c", shcmd)
	c.Stdout = &buf
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return err
	}
	b = buf.Bytes()
	if _, err := parseAptURIsList(b); err != nil {
		return fmt.Errorf("bug: lock file generated from docker container is invalid: %v", err)
	}
	return ioutil.WriteFile(*aptPinFile, buf.Bytes(), 0644)
}

func runAptInstall(cmd *cobra.Command, args []string) error {
	b, err := ioutil.ReadFile(*aptPinFile)
	if err != nil {
		return fmt.Errorf("failed to read pin file %q: %v", *aptPinFile, err)
	}
	pkgs, err := parseAptURIsList(b)
	if err != nil {
		return fmt.Errorf("failed to parse pin file: %v", err)
	}

	if len(pkgs) == 0 {
		fmt.Fprintf(os.Stderr, "No packages in the lock file, nothing to be done\n")
		return nil
	}

	var files []string
	for _, p := range pkgs {
		f, err := fetchPackage(p)
		if err != nil {
			return err
		}
		files = append(files, f)
		defer os.Remove(f)
	}

	dpkgArgs := append([]string{"-i"}, files...)
	c := exec.Command("dpkg", dpkgArgs...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

func fetchPackage(p AptPackage) (string, error) {
	target := filepath.Join("/var/cache/apt/archives", p.Filename)
	if _, err := os.Stat(target); err == nil { // Return immediately if the file exists.
		return target, nil
	}
	fh, err := os.OpenFile(filepath.Join("/var/cache/apt/archives/partial", p.Filename), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return "", err
	}
	fmt.Fprintf(os.Stderr, "Downloading %s... (%s)\n", p.URL, humanize.IBytes(uint64(p.Size)))
	resp, err := http.Get(p.URL)
	if err != nil {
		return "", fmt.Errorf("failed to download %q: %v", p.URL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("failed to download %q: HTTP %s", p.URL, resp.Status)
	}
	h := md5.New()
	n, err := io.Copy(io.MultiWriter(fh, h), resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to download %q: %v", p.URL, err)
	}
	if n != p.Size {
		return "", fmt.Errorf("size mismatch for %q: %d instead of %d", p.URL, n, p.Size)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if sum != p.MD5 {
		return "", fmt.Errorf("hash mismatch for %q: %q instead of %q", p.URL, sum, p.MD5)
	}
	if err := fh.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(fh.Name(), target); err != nil {
		return "", err
	}
	return target, nil
}

type AptPackage struct {
	URL      string
	Filename string
	Size     int64
	MD5      string
}

var aptUriRe = regexp.MustCompile(`^'([^']+)' (\S+) (\d+) MD5Sum:([0-9a-f]{32})`)

func parseAptURIsList(b []byte) ([]AptPackage, error) {
	var ret []AptPackage
	for _, l := range bytes.Split(b, []byte{'\n'}) {
		s := string(l)
		if strings.HasPrefix(s, "#") || s == "" {
			continue
		}
		if strings.HasPrefix(s, "base-image=") {
			// TODO: Use this
			continue
		}
		m := aptUriRe.FindStringSubmatch(s)
		if m == nil {
			return nil, fmt.Errorf("failed to parse line %q", s)
		}
		p := AptPackage{
			URL:      m[1],
			Filename: m[2],
			MD5:      m[4],
		}
		p.Size, _ = strconv.ParseInt(m[3], 10, 64)
		ret = append(ret, p)
	}
	return ret, nil
}
