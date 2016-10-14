package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/kr/pretty"
	"github.com/pedrommone/sentry-mttr-mtbf-calculator/log"
	"github.com/Sirupsen/logrus"
	"github.com/tomnomnom/linkheader"

	_ "github.com/joho/godotenv/autoload"
)

type Calculator struct {
	Log		*logrus.Logger
}

type Organization struct {
	Id		string `json:"id"`
	Name		string `json:"name"`
	Slug		string `json:"slug"`
}

type Project struct {
	Name		string `json:"name,omitempty"`
	Slug		string `json:"slug,omitempty"`
	Organization	Organization
}

type Issue struct {
	Id		string `json:"id,omitempty"`
	Status		string `json:"status,omitempty"`
	Project		Project
	Activity		[]Activity
}

type Activity struct {
	Id		string `json:"id,omitempty"`
	DateCreated	string `json:"dateCreated,omitempty"`
 	Type		string `json:"type,omitempty"`
}

const sentryURL = "https://sentry.io/api/"

var (
	sentryToken	string
	projects	[]Project
	issues		[]Issue
)

func main() {
	sentryToken = os.Getenv("SENTRY_TOKEN")

	if sentryToken == "" {
		panic("Sentry token need.")
	}

	calculator := NewCalculator()
	calculator.Start()
}

func NewCalculator() *Calculator {
	calc := new(Calculator)
	calc.Log = log.NewLogrus()

	return calc;
}

func (c *Calculator) Start() {
	projects = append(projects, c.getProjects("0:0:0")...)

	for _, project := range projects {
		issues = append(issues, c.getIssues(project, "0:0:0")...)
	}

	fmt.Print(fmt.Sprintf("%# v", pretty.Formatter(issues)))
}

func (c *Calculator) requestProjects(cursor string) (resp *http.Response, err error) {
	client := &http.Client {}
	uri := fmt.Sprintf("%s0/projects/?query=&cursor=%s", sentryURL, cursor)

	c.Log.Info(fmt.Sprintf("GET %s", uri))

	req, _ := http.NewRequest("GET", uri, nil);
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", sentryToken))

	resp, err = client.Do(req)

	if err != nil {
		panic("Error while fetch data.")
	}

	return
}

func (c *Calculator) getProjects(cursor string) (projects []Project) {
	resp, _ := c.requestProjects(cursor)

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(b, &projects)
	if err != nil {
		panic(err)
	}

	link := resp.Header.Get("Link")
	links := linkheader.Parse(link)
	nextPage := links[1].Params

	if nextPage["results"] == "true" {
		c.getProjects(nextPage["cursor"])
	}

	return
}

func (c *Calculator) requestIssues(project Project, cursor string) (resp *http.Response, err error) {
	client := &http.Client {}
	uri := fmt.Sprintf("%s0/projects/%s/%s/issues/?query=&cursor=%s", sentryURL, project.Organization.Slug, project.Slug, cursor)

	c.Log.Info(fmt.Sprintf("GET %s", uri))

	req, _ := http.NewRequest("GET", uri, nil);
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", sentryToken))

	resp, err = client.Do(req)

	if err != nil {
		panic("Error while fetch data.")
	}

	return
}

func (c *Calculator) getIssues(project Project, cursor string) (issues []Issue) {
	resp, _ := c.requestIssues(project, cursor)
	currentIssues := []Issue {}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(b, &currentIssues)
	if err != nil {
		panic(err)
	}

	for _, row := range currentIssues {
		issues = append(issues, c.getIssue(row.Id))
	}

	link := resp.Header.Get("Link")
	links := linkheader.Parse(link)
	nextPage := links[1].Params

	if nextPage["results"] == "true" {
		c.getIssues(project, nextPage["cursor"])
	}

	return
}

func (c *Calculator) getIssue(id string) (issue Issue) {
	resp, _ := c.requestIssue(id)

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(b, &issue)
	if err != nil {
		panic(err)
	}

	return
}

func (c *Calculator) requestIssue(id string) (resp *http.Response, err error) {
	client := &http.Client {}
	uri := fmt.Sprintf("%s0/issues/%s/", sentryURL, id)

	c.Log.Info(fmt.Sprintf("GET %s", uri))

	req, _ := http.NewRequest("GET", uri, nil);
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", sentryToken))

	resp, err = client.Do(req)

	if err != nil {
		panic("Error while fetch data.")
	}

	return
}
