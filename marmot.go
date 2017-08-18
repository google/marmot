// Copyright 2017 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"log"
	"os"

	"github.com/golang/glog"
	"github.com/johnsiilver/golib/http/server"
	"github.com/google/marmot/instance"
	flag "github.com/spf13/pflag"
)

var (
	sport      = flag.Int32("sport", 0, "Port to run the grpc service on. Defaults to random.")
	storage    = flag.String("storage", "", "The type of storage to use. Valid: 'inmemory'")
	maxCrashes = flag.Int("max_crashes", 3, "The number of times a cog can fail and be restarted")
)

var (
	insecure = flag.Bool("insecure", false, "Run without TLS.  NOT RECOMMENDED!!!")
	certFile = flag.String("cert_file", "", "The location of the certificate file for TLS")
	keyFile  = flag.String("key_file", "", "the location of the key file for TLS")
)

var (
	userPass = flag.Bool("user_pass_auth", false, "Sets the server to use user/password authentication instead of client side certificates")
	authFile = flag.String("auth_file", "", "The location of the authentication file")
)

// These flags are for the webserver.
var (
	runWeb = flag.Bool("webserver", false, "Indicates if you wish to run a webserver")
	hport  = flag.Int32("hport", 0, "Port to run the webserver on. Defaults to random.")
	errLog = flag.String("http_errorlog", "", "The file path to write the http server's error log. By default, we do not write one")
)

var validStorage = map[string]bool{
	"inmemory": true,
}

func validateFlags() error {
	if !validStorage[*storage] {
		return fmt.Errorf("-storage value %q is invalid", *storage)
	}
	if *sport < 0 {
		return fmt.Errorf("-sport cannot be < 0: %d", *sport)
	}

	if !*insecure {
		switch "" {
		case *certFile, *keyFile:
			return fmt.Errorf("-cert_file and --key_file cannot be set to empty string")
		}
	}

	if *authFile == "" {
		return fmt.Errorf("-auth_file cannot be empty string")
	}

	return nil
}

func main() {
	flag.Parse()

	glog.Infof("starting...")

	if err := validateFlags(); err != nil {
		panic(err)
	}

	args := instance.Args{
		SPort:      *sport,
		MaxCrashes: *maxCrashes,
		Insecure:   *insecure,
		CertFile:   *certFile,
		KeyFile:    *keyFile,
		Storage:    *storage,
	}

	marmot, err := instance.New(args)
	if err != nil {
		panic(err)
	}

	// TODO(johnsiilver): Add server options to pass.
	if *errLog != "" {
		f, err := os.OpenFile(*errLog, os.O_RDWR|os.O_CREATE, 0666)
		if err != nil {
			panic(fmt.Sprintf("could not open log file at %s as specified by --http_errorlog: %s", *errLog, err))
		}
		server.ErrorLog(log.New(f, "", log.LstdFlags|log.Lshortfile))
	}

	marmot.Run()
}
