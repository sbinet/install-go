package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

var g_odir = flag.String("o", "/afs/cern.ch/sw/lcg/contrib/go", "output directory where to install the go runtime")
var g_verbose = flag.Bool("v", false, "enable verbose printouts")
var g_mode = flag.String("mode", "curl", "which mode to use (go|curl)")

const (
	url_tmpl = "http://golang.org/dl/go%s.%s-%s.tar.gz"
)

func curl_download(version string, platform [2]string) error {
	odir := filepath.Join(*g_odir, version, "tmp-"+platform[0]+"-"+platform[1])

	// create out-dir layout
	if *g_verbose {
		fmt.Printf("~~~ %s\n", odir)
	}
	err := os.MkdirAll(odir, 0755)
	if err != nil {
		return err
	}

	// http://golang.org/dl/go1.2.2.linux-amd64.tar.gz
	url := fmt.Sprintf(
		url_tmpl,
		version,
		platform[0],
		platform[1],
	)

	curl := exec.Command(
		"curl",
		"-L", url,
	)

	untar := exec.Command(
		"tar",
		"-C", odir,
		"-zxf",
		"-",
	)

	if *g_verbose {
		fmt.Printf("curl.cmd:    %v\n", curl.Args)
		fmt.Printf("untar.cmd:   %v\n", untar.Args)
	}

	curl_out, err := curl.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "**error** curl.cmd:    %v\n", curl.Args)
		fmt.Fprintf(os.Stderr, "**error** curl.stdout: %v\n", err)
		return err
	}

	err = curl.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "**error** curl.cmd:   %v\n", curl.Args)
		fmt.Fprintf(os.Stderr, "**error** curl.start: %v\n", err)
		return err
	}

	untar.Stdin = curl_out

	err = untar.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "**error** untar.cmd:   %v\n", untar.Args)
		fmt.Fprintf(os.Stderr, "**error** untar.start: %v\n", err)
		return err
	}

	err = curl.Wait()
	if err != nil {
		fmt.Fprintf(os.Stderr, "**error** curl.cmd:   %v\n", curl.Args)
		fmt.Fprintf(os.Stderr, "**error** curl.start: %v\n", err)
		return err
	}

	err = untar.Wait()
	if err != nil {
		fmt.Fprintf(os.Stderr, "**error** untar.cmd:  %v\n", untar.Args)
		fmt.Fprintf(os.Stderr, "**error** untar.wait: %v\n", err)
		return err
	}

	err = os.Rename(
		filepath.Join(odir, "go"),
		filepath.Join(*g_odir, version, platform[0]+"_"+platform[1]),
	)
	if err != nil {
		return err
	}
	err = os.RemoveAll(odir)
	if err != nil {
		return err
	}

	goroot := filepath.Join(*g_odir, version, platform[0]+"_"+platform[1])
	return create_setup_files(goroot, platform)
}

func go_download(version string, platform [2]string) error {
	odir := filepath.Join(*g_odir, version, "tmp-"+platform[0]+"-"+platform[1])

	// create out-dir layout
	if *g_verbose {
		fmt.Printf("~~~ %s\n", odir)
	}
	err := os.MkdirAll(odir, 0755)
	if err != nil {
		return err
	}

	// http://golang.org/dl/go1.1.2.linux-amd64.tar.gz
	url := fmt.Sprintf(
		url_tmpl,
		version,
		platform[0],
		platform[1],
	)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	// Iterate through the files in the archive.
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "**error** %v\n", err)
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			dir := filepath.Join(odir, hdr.Name)
			if *g_verbose {
				fmt.Printf(">>> %s\n", dir)
			}
			err = os.MkdirAll(dir, 0755)
			if err != nil {
				return err
			}
			continue

		case tar.TypeReg, tar.TypeRegA:
			// ok
		default:
			fmt.Fprintf(os.Stderr, "**error: %v\n", hdr.Typeflag)
			return err
		}
		oname := filepath.Join(odir, hdr.Name)
		if *g_verbose {
			fmt.Printf("::: %s (%s)\n", oname, string(byte(hdr.Typeflag)))
		}
		dir := filepath.Dir(oname)
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			return err
		}

		o, err := os.OpenFile(oname,
			os.O_WRONLY|os.O_CREATE,
			os.FileMode(hdr.Mode),
		)
		if err != nil {
			return err
		}
		defer o.Close()
		_, err = io.Copy(o, tr)
		if err != nil {
			return err
		}
		o.Sync()
		o.Close()
	}

	err = os.Rename(
		filepath.Join(odir, "go"),
		filepath.Join(*g_odir, version, platform[0]+"_"+platform[1]),
	)
	if err != nil {
		return err
	}
	err = os.RemoveAll(odir)
	if err != nil {
		return err
	}

	goroot := filepath.Join(*g_odir, version, platform[0]+"_"+platform[1])
	return create_setup_files(goroot, platform)
}

func create_setup_files(goroot string, platform [2]string) error {
	// create setup.sh
	setup_sh, err := os.Create(
		filepath.Join(goroot, "setup.sh"),
	)
	if err != nil {
		return err
	}
	defer setup_sh.Close()
	_, err = setup_sh.WriteString(fmt.Sprintf(`#!/bin/sh

export GOROOT=%s
export PATH=${GOROOT}/bin:${PATH}
export GOPATH=${HOME}/dev/gocode
export PATH=${GOPATH}/bin:${PATH}
`, goroot))
	if err != nil {
		return err
	}

	// create setup.csh
	setup_csh, err := os.Create(
		filepath.Join(goroot, "setup.csh"),
	)
	if err != nil {
		return err
	}
	defer setup_csh.Close()
	_, err = setup_csh.WriteString(fmt.Sprintf(`
setenv GOROOT %s
setenv PATH ${GROOT}/bin:${PATH}
setenv GOPATH ${HOME}/dev/gocode
setenv PATH ${GOPATH}/bin:${PATH}
`))
	if err != nil {
		return err
	}

	return err
}

func main() {
	flag.Parse()
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `atl-install-go installs the go-gc runtime.
Usage: 

$ atl-install-go [options] <go-version>
`)
		flag.PrintDefaults()
	}

	if flag.NArg() <= 0 {
		flag.Usage()
		os.Exit(1)
	}

	version := flag.Arg(0)
	if version == "" {
		flag.Usage()
		os.Exit(1)
	}

	platforms := [][2]string{
		{"linux", "amd64"},
		{"linux", "386"},
	}

	type download_fct func(version string, platform [2]string) error
	download := go_download

	switch *g_mode {
	case "curl":
		download = curl_download
	case "go":
		download = go_download
	default:
		fmt.Fprintf(os.Stderr, "**error** invalid download mode (%s)\n", *g_mode)
	}

	all_good := true
	errch := make(chan error, len(platforms))
	for _, plat := range platforms {
		go func(plat [2]string) {
			fmt.Printf(":: installing %s-%s...\n", plat[0], plat[1])
			err := download(version, plat)
			errch <- err
			if err != nil {
				return
			}
			fmt.Printf(":: installing %s-%s... [ok]\n", plat[0], plat[1])
		}(plat)
	}

	for i := 0; i < len(platforms); i++ {
		err := <-errch
		if err != nil {
			all_good = false
			fmt.Fprintf(os.Stderr, "**error** %v\n", err)
		}
		if i == len(platforms)-1 {
			close(errch)
		}
	}

	if !all_good {
		os.Exit(1)
	}
}

// EOF
