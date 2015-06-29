package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/codegangsta/cli"
)

const (
	defaultHost  = "dorking.cs.dixie.edu"
	cookiePrefix = "codegrinder_session="
	rcFile       = ".codegrinderrc"
)

var Config struct {
	Cookie string
	Host   string
}

func getAllFiles() map[string]string {
	// gather all the files in the current directory
	files := make(map[string]string)
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("walk error for %s: %v", path, err)
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasPrefix(path, ".") {
			return nil
		}
		contents, err := ioutil.ReadFile(path)
		if err != nil {
			log.Printf("error loading %s: %v", path, err)
			return err
		}
		log.Printf("found %s with %d bytes", path, len(contents))
		files[path] = string(contents)
		return nil
	})
	if err != nil {
		log.Fatalf("walk error: %v", err)
	}
	return files
}

func main() {
	log.SetFlags(log.Ltime)
	app := cli.NewApp()
	app.Name = "grind"
	app.Usage = "command-line interface to codegrinder"
	app.Version = "0.0.1"
	app.Authors = []cli.Author{
		{Name: "Russ Ross", Email: "russ@russross.com"},
	}
	app.Commands = []cli.Command{
		{
			Name:   "init",
			Usage:  "connect to codegrinder server",
			Action: CommandInit,
		},
		{
			Name:   "create",
			Usage:  "create a new problem (instructors only)",
			Action: CommandCreate,
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "update",
					Usage: "update an existing problem",
				},
			},
		},
	}
	app.Run(os.Args)

	/*
		// create a websocket connection to the server
		headers := make(http.Header)
		socket, resp, err := websocket.DefaultDialer.Dial("ws://dorking.cs.dixie.edu:8080/python2unittest", headers)
		if err != nil {
			log.Printf("websocket dial: %v", err)
			if resp != nil && resp.Body != nil {
				io.Copy(os.Stderr, resp.Body)
				resp.Body.Close()
			}
			log.Fatalf("giving up")
		}

			// get the files to submit
			var action Action
			action.Type = "python2unittest"
			action.Files = getAllFiles()
			if err := socket.WriteJSON(&action); err != nil {
				log.Fatalf("error writing Action message: %v", err)
			}

			// start listening for events
			for {
				var event EventMessage
				if err := socket.ReadJSON(&event); err != nil {
					log.Printf("socket error reading event: %v", err)
					break
				}
				fmt.Print(event.StreamData)
			}
			socket.Close()
			log.Printf("quitting")
	*/
}

func CommandInit(context *cli.Context) {
	fmt.Println(
		`Please follow these steps:

1.  Use Canvas to load a CodeGrinder window

2.  Open a new tab in your browser and copy this URL into the address bar:

    https://` + defaultHost + `/api/v2/users/me/cookie

3.  The browser will display something of the form:

    ` + cookiePrefix + `...

4.  Copy that entire string to the clipboard and paste it below.

Paste here: `)

	var cookie string
	n, err := fmt.Scanln(&cookie)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error encountered while reading the cookie you pasted: %v\n", err)
		os.Exit(1)
	}
	if n != 1 {
		fmt.Fprintf(os.Stderr, "failed to read the cookie you pasted; please try again\n")
		os.Exit(1)
	}
	if !strings.HasPrefix(cookie, cookiePrefix) {
		fmt.Fprintf(os.Stderr, "the cookie must start with %s; perhaps you copied the wrong thing?\n", cookiePrefix)
		os.Exit(1)
	}

	// set up config
	Config.Cookie = cookie
	Config.Host = defaultHost

	// try it out by fetching a user record
	user := new(User)
	mustGetObject("/users/me", nil, user)

	// save config for later use
	mustWriteConfig()

	raw, err := json.MarshalIndent(user, "", "    ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON error encoding user record: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("%s\n", raw)
}

func mustGetObject(path string, params map[string]string, download interface{}) {
	mustRequest(path, params, Config.Cookie, "GET", nil, download)
}

func mustPostObject(path string, params map[string]string, upload interface{}, download interface{}) {
	mustRequest(path, params, Config.Cookie, "POST", upload, download)
}

func mustPutObject(path string, params map[string]string, upload interface{}, download interface{}) {
	mustRequest(path, params, Config.Cookie, "PUT", upload, download)
}

func mustRequest(path string, params map[string]string, cookie string, method string, upload interface{}, download interface{}) {
	if !strings.HasPrefix(path, "/") {
		log.Panicf("mustRequest path must start with /")
	}
	if method != "GET" && method != "POST" && method != "PUT" && method != "DELETE" {
		log.Panicf("mustRequest only recognizes GET, POST, PUT, and DELETE methods")
	}
	req, err := http.NewRequest(method, fmt.Sprintf("https://%s/api/v2%s", Config.Host, path), nil)
	if err != nil {
		log.Fatalf("error creating http request: %v\n", err)
	}

	// add any parameters
	if params != nil && len(params) > 0 {
		values := req.URL.Query()
		for key, value := range params {
			values.Add(key, value)
		}
		req.URL.RawQuery = values.Encode()
	}

	// set the headers
	req.Header["Accept"] = []string{"application/json"}
	req.Header["Cookie"] = []string{cookie}

	// upload the payload if any
	if upload != nil && (method == "POST" || method == "PUT") {
		req.Header["Content-Type"] = []string{"application/json"}
		payload, err := json.MarshalIndent(upload, "", "    ")
		if err != nil {
			log.Fatalf("mustPostObject: JSON error encoding object to upload: %v", err)
		}
		req.Body = ioutil.NopCloser(bytes.NewReader(payload))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("error connecting to %s: %v\n", Config.Host, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("unexpected status from %s: %s\n", Config.Host, resp.Status)
		io.Copy(os.Stderr, resp.Body)
		log.Fatalf("giving up")
	}

	// parse the result if any
	if download != nil {
		decoder := json.NewDecoder(resp.Body)
		if err := decoder.Decode(download); err != nil {
			log.Fatalf("failed to parse result object from server: %v\n", err)
		}
	}
}

func mustLoadConfig() {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	if home == "" {
		fmt.Fprintf(os.Stderr, "Unable to locate home directory, giving up\n")
		os.Exit(1)
	}
	configFile := filepath.Join(home, rcFile)

	if raw, err := ioutil.ReadFile(configFile); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to load config file; try running \"grind init\"\n")
		os.Exit(1)
	} else {
		if err := json.Unmarshal(raw, &Config); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse %s: %v", configFile, err)
			fmt.Fprintf(os.Stderr, "you may wish to try deleting the file and running \"grind init\" again\n")
			os.Exit(1)
		}
	}
}

func mustWriteConfig() {
	home := os.Getenv("HOME")
	if home == "" {
		home = os.Getenv("USERPROFILE")
	}
	if home == "" {
		fmt.Fprintf(os.Stderr, "Unable to locate home directory, giving up\n")
		os.Exit(1)
	}
	configFile := filepath.Join(home, rcFile)

	raw, err := json.MarshalIndent(&Config, "", "    ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "JSON error encoding cookie file: %v", err)
		os.Exit(1)
	}
	raw = append(raw, '\n')

	if err = ioutil.WriteFile(configFile, raw, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v", configFile, err)
		os.Exit(1)
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
