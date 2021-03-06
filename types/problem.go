package types

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"net/url"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/russross/blackfriday"
	"golang.org/x/net/html"
)

var BeginningOfTime = time.Date(2016, 1, 1, 0, 0, 0, 0, time.UTC)

// ProblemType defines one type of problem.
type ProblemType struct {
	Name        string                        `json:"name"`
	Image       string                        `json:"image"`
	MaxCPU      int                           `json:"maxCPU"`
	MaxClock    int                           `json:"maxClock"`
	MaxFD       int                           `json:"maxFD"`
	MaxFileSize int                           `json:"maxFileSize"`
	MaxMemory   int                           `json:"maxMemory"`
	MaxThreads  int                           `json:"maxThreads"`
	Actions     map[string]*ProblemTypeAction `json:"actions"`
	Files       map[string]string             `json:"files,omitempty"`
}

// ProblemTypeAction defines the label, button, UI classes, and handler for a
// single problem type action.
type ProblemTypeAction struct {
	Action  string `json:"action,omitempty"`
	Button  string `json:"button,omitempty"`
	Message string `json:"message,omitempty"`
	Class   string `json:"className,omitempty"`
	Handler interface{}
}

type Problem struct {
	ID          int64     `json:"id" meddler:"id,pk"`
	Unique      string    `json:"unique" meddler:"unique_id"`
	Note        string    `json:"note" meddler:"note"`
	ProblemType string    `json:"problemType" meddler:"problem_type"`
	Tags        []string  `json:"tags" meddler:"tags,json"`
	Options     []string  `json:"options" meddler:"options,json"`
	CreatedAt   time.Time `json:"createdAt" meddler:"created_at,localtime"`
	UpdatedAt   time.Time `json:"updatedAt" meddler:"updated_at,localtime"`
}

// ProblemStep represents a single step of a problem.
// Anything in the root directory of Files is added to the working directory,
// possibly overwriting existing content. The subdirectory contents of Files
// replace all subdirectory contents in the problem from earlier steps.
type ProblemStep struct {
	ProblemID    int64             `json:"problemID" meddler:"problem_id"`
	Step         int64             `json:"step" meddler:"step"` // note: one-based
	Note         string            `json:"note" meddler:"note"`
	Instructions string            `json:"instructions" meddler:"instructions"`
	Weight       float64           `json:"weight" meddler:"weight"`
	Files        map[string]string `json:"files" meddler:"files,json"`
}

type ProblemSet struct {
	ID        int64     `json:"id" meddler:"id,pk"`
	Unique    string    `json:"unique" meddler:"unique_id"`
	Note      string    `json:"note" meddler:"note"`
	Tags      []string  `json:"tags" meddler:"tags,json"`
	CreatedAt time.Time `json:"createdAt" meddler:"created_at,localtime"`
	UpdatedAt time.Time `json:"updatedAt" meddler:"updated_at,localtime"`
}

type ProblemSetProblem struct {
	ProblemSetID int64   `json:"problemSetID" meddler:"problem_set_id"`
	ProblemID    int64   `json:"problemID" meddler:"problem_id"`
	Weight       float64 `json:"weight" meddler:"weight"`
}

func (problem *Problem) Normalize(now time.Time, steps []*ProblemStep) error {
	// make sure the unique ID is valid
	problem.Unique = strings.TrimSpace(problem.Unique)
	if problem.Unique == "" {
		return fmt.Errorf("unique ID cannot be empty")
	}
	if url.QueryEscape(problem.Unique) != problem.Unique {
		return fmt.Errorf("unique ID must be URL friendly: %s is escaped as %s",
			problem.Unique, url.QueryEscape(problem.Unique))
	}

	// make sure the note is valid
	problem.Note = strings.TrimSpace(problem.Note)
	if problem.Note == "" {
		return fmt.Errorf("note cannot be empty")
	}

	// 	// make sure the problem type is legitimate
	// 	if _, exists := problemTypes[problem.ProblemType]; !exists {
	// 		return fmt.Errorf("unrecognized problem type: %q", problem.ProblemType)
	// 	}

	// check tags
	for i, tag := range problem.Tags {
		problem.Tags[i] = strings.TrimSpace(tag)
	}
	sort.Strings(problem.Tags)

	// check options
	for i, option := range problem.Options {
		problem.Options[i] = strings.TrimSpace(option)
	}

	// check steps
	if len(steps) == 0 {
		return fmt.Errorf("problem must have at least one step")
	}
	for n, step := range steps {
		step.Normalize(int64(n) + 1)
	}

	// sanity check timestamps
	if problem.CreatedAt.Before(BeginningOfTime) || problem.CreatedAt.After(now) {
		return fmt.Errorf("problem CreatedAt time of %v is invalid", problem.CreatedAt)
	}
	if problem.UpdatedAt.Before(problem.CreatedAt) || problem.UpdatedAt.After(now) {
		return fmt.Errorf("problem UpdatedAt time of %v is invalid", problem.UpdatedAt)
	}

	return nil
}

func (problem *Problem) ComputeSignature(secret string, steps []*ProblemStep) string {
	v := make(url.Values)

	// gather all relevant fields
	v.Add("id", strconv.FormatInt(problem.ID, 10))
	v.Add("unique", problem.Unique)
	v.Add("note", problem.Note)
	v.Add("problemType", problem.ProblemType)
	v["tags"] = problem.Tags
	v["options"] = problem.Options
	v.Add("createdAt", problem.CreatedAt.Round(time.Second).UTC().Format(time.RFC3339))
	v.Add("updatedAt", problem.UpdatedAt.Round(time.Second).UTC().Format(time.RFC3339))
	for _, step := range steps {
		v.Add(fmt.Sprintf("step-%d-note", step.Step), step.Note)
		v.Add(fmt.Sprintf("step-%d-weight", step.Step), strconv.FormatFloat(step.Weight, 'g', -1, 64))
		for name, contents := range step.Files {
			v.Add(fmt.Sprintf("step-%d-file-%s", step.Step, name), contents)
		}
	}

	// compute signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(encode(v)))
	sum := mac.Sum(nil)
	sig := base64.StdEncoding.EncodeToString(sum)
	//log.Printf("problem signature: %s data: %s", sig, encode(v))
	return sig
}

// problem files in these directories do not have line endings cleaned up
var ProblemStepDirectoryWhitelist = map[string]bool{
	"in":   true,
	"out":  true,
	"_doc": true,
}

// fix line endings
func (step *ProblemStep) Normalize(n int64) error {
	step.Step = n
	step.Note = strings.TrimSpace(step.Note)
	if step.Note == "" {
		return fmt.Errorf("missing note for step %d", n+1)
	}
	instructions, err := step.BuildInstructions()
	if err != nil {
		return fmt.Errorf("error building instructions for step %d: %v", n+1, err)
	}
	step.Instructions = instructions
	if step.Weight <= 0.0 {
		// default to 1.0
		step.Weight = 1.0
	}
	clean := make(map[string]string)
	for name, contents := range step.Files {
		parts := strings.Split(name, "/")
		fixed := contents
		if (len(parts) < 2 || !ProblemStepDirectoryWhitelist[parts[0]]) && utf8.ValidString(contents) {
			fixed = fixLineEndings(contents)
			if fixed != contents {
				log.Printf("fixed line endings for %s", name)
			}
		} else if utf8.ValidString(contents) {
			fixed = fixNewLines(contents)
			if fixed != contents {
				log.Printf("fixed newlines for %s", name)
			}
		}
		clean[name] = fixed
	}
	step.Files = clean
	return nil
}

func (problem *Problem) GetStepWhitelists(steps []*ProblemStep) []map[string]bool {
	var lists []map[string]bool

	// compute the white list of commit files for each step
	for _, step := range steps {
		// carry everything forward
		m := make(map[string]bool)
		if len(lists) > 0 {
			for name := range lists[len(lists)-1] {
				m[name] = true
			}
		}

		// add files defined in the root directory of the problem step
		for name := range step.Files {
			if len(strings.Split(name, "/")) == 1 {
				m[name] = true
			}
		}
		lists = append(lists, m)
	}

	return lists
}

// buildInstructions builds the instructions for a problem step as a single
// html document. Markdown is processed and images are inlined.
func (step *ProblemStep) BuildInstructions() (string, error) {
	// get a list of all files in the _doc directory
	used := make(map[string]bool)
	for name := range step.Files {
		if strings.HasPrefix(name, "_doc/") {
			used[name] = false
		}
	}

	var justHTML string
	if data, ok := step.Files["_doc/index.html"]; ok {
		justHTML = data
		used["_doc/index.html"] = true
	} else if data, ok := step.Files["_doc/index.md"]; ok {
		// render markdown
		extensions := 0
		extensions |= blackfriday.EXTENSION_NO_INTRA_EMPHASIS
		extensions |= blackfriday.EXTENSION_TABLES
		extensions |= blackfriday.EXTENSION_FENCED_CODE
		extensions |= blackfriday.EXTENSION_AUTOLINK
		extensions |= blackfriday.EXTENSION_STRIKETHROUGH
		extensions |= blackfriday.EXTENSION_SPACE_HEADERS

		justHTML = string(blackfriday.Markdown([]byte(data), blackfriday.HtmlRenderer(0, "", ""), extensions))
		used["_doc/index.md"] = true
	} else {
		return "", loggedErrorf("No documentation found: checked _doc/index.html and _doc/index.md")
	}

	// make sure it is well-formed utf8
	if !utf8.ValidString(justHTML) {
		return "", loggedErrorf("index.{html,md} is not valid utf8")
	}

	// parse the html
	doc, err := html.Parse(strings.NewReader(justHTML))
	if err != nil {
		log.Printf("Error parsing index.html: %v", err)
		return "", err
	}
	if doc == nil {
		return "", loggedErrorf("Parsing the HTML yielded a nil document")
	}

	// find image tags
	var walk func(*html.Node) error
	walk = func(n *html.Node) error {
		if n.Type == html.ElementNode && n.Data == "img" {
			for i, a := range n.Attr {
				if a.Key == "src" {
					if contents, present := step.Files["_doc/"+a.Val]; present {
						mime := ""
						switch {
						case strings.HasSuffix(a.Val, ".gif"):
							mime = "image/gif"
						case strings.HasSuffix(a.Val, ".png"):
							mime = "image/png"
						case strings.HasSuffix(a.Val, ".jpg"):
							mime = "image/jpeg"
						case strings.HasSuffix(a.Val, ".jpeg"):
							mime = "image/jpeg"
						case strings.HasSuffix(a.Val, ".svg"):
							mime = "image/svg+xml"
						default:
							return loggedErrorf("image tag found, but image type is unknown: %s", a.Val)
						}

						// base64 encode the image
						log.Printf("encoding image %s as base64 data URI", a.Val)
						used["_doc/"+a.Val] = true
						s := base64.StdEncoding.EncodeToString([]byte(contents))
						a.Val = fmt.Sprintf("data:%s;base64,%s", mime, s)
						n.Attr[i] = a
					} else {
						return loggedErrorf("Warning: image tag found, but image file not found: %s", a.Val)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if err := walk(c); err != nil {
				return err
			}
		}
		return nil
	}
	if err = walk(doc); err != nil {
		return "", err
	}

	// warn about unused files in _doc
	for name, u := range used {
		if !u {
			log.Printf("Warning: %s was not used in the instructions", name)
		}
	}

	// re-render it
	var buf bytes.Buffer
	if err = html.Render(&buf, doc); err != nil {
		log.Printf("Error rendering HTML: %v", err)
		return "", err
	}

	return buf.String(), nil
}

func (set *ProblemSet) Normalize(now time.Time) error {
	// make sure the unique ID is valid
	set.Unique = strings.TrimSpace(set.Unique)
	if set.Unique == "" {
		return fmt.Errorf("unique ID cannot be empty")
	}
	if url.QueryEscape(set.Unique) != set.Unique {
		return fmt.Errorf("unique ID must be URL friendly: %s is escaped as %s",
			set.Unique, url.QueryEscape(set.Unique))
	}

	// make sure the note is valid
	set.Note = strings.TrimSpace(set.Note)
	if set.Note == "" {
		return fmt.Errorf("note cannot be empty")
	}

	// check tags
	for i, tag := range set.Tags {
		set.Tags[i] = strings.TrimSpace(tag)
	}
	sort.Strings(set.Tags)

	// sanity check timestamps
	if set.CreatedAt.Before(BeginningOfTime) || set.CreatedAt.After(now) {
		return fmt.Errorf("problem set CreatedAt time of %v is invalid", set.CreatedAt)
	}
	if set.UpdatedAt.Before(set.CreatedAt) || set.UpdatedAt.After(now) {
		return fmt.Errorf("problem set UpdatedAt time of %v is invalid", set.UpdatedAt)
	}

	return nil
}

func fixLineEndings(s string) string {
	s = strings.Replace(s, "\r\n", "\n", -1) + "\n"
	for strings.Contains(s, " \n") {
		s = strings.Replace(s, " \n", "\n", -1)
	}
	for strings.HasSuffix(s, "\n\n") {
		s = s[:len(s)-1]
	}
	if s == "\n" {
		s = ""
	}
	return s
}

func fixNewLines(s string) string {
	s = strings.Replace(s, "\r\n", "\n", -1) + "\n"
	for strings.HasSuffix(s, "\n\n") {
		s = s[:len(s)-1]
	}
	if s == "\n" {
		s = ""
	}
	return s
}

func loggedErrorf(f string, params ...interface{}) error {
	log.Print(logPrefix() + fmt.Sprintf(f, params...))
	return fmt.Errorf(f, params...)
}

func logPrefix() string {
	prefix := ""
	if _, file, line, ok := runtime.Caller(2); ok {
		short := file
		for i := len(file) - 1; i > 0; i-- {
			if file[i] == '/' {
				short = file[i+1:]
				break
			}
		}
		file = short
		prefix = fmt.Sprintf("%s:%d: ", file, line)
	}
	return prefix
}
