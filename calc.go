package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/kr/pretty"
	"github.com/pedrommone/sentry-mttr-mtbf-calculator/log"
	"github.com/Sirupsen/logrus"
	"github.com/tealeg/xlsx"
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

type ComputedActivity struct {
	Issue		Issue
	Duration	float64
}

const (
	sentryURL	= "https://sentry.io/api/"
	timeFormat	= "2006-01-02T15:04:05Z07:00"
	sheetName	= "result.xlsx"
)

var (
	sentryToken	string
	projects	[]Project
	issues		[]Issue
	activities	[]ComputedActivity
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

	return calc
}

func (c *Calculator) Start() {
	projects = append(projects, c.getProjects("0:0:0")...)

	for _, project := range projects {
		issues = append(issues, c.getIssues(project, "0:0:0")...)
	}

	c.Log.Debug("====================")
	c.Log.Debug("Dataset")
	c.Log.Debug(fmt.Sprintf("%# v", pretty.Formatter(issues)))
	c.Log.Debug("====================")

	mttr := c.calcMTTR(issues)
	c.Log.Info(fmt.Sprintf("MTTR: %.2f minutes", mttr))

	c.saveActivitiesIntoXLSX(activities)
}

func (c *Calculator) saveActivitiesIntoXLSX(activities []ComputedActivity) {
	var file *xlsx.File
	var sheet *xlsx.Sheet
	var row *xlsx.Row
	var cell *xlsx.Cell
	var err error

	totalActivities := len(activities)
	c.Log.Info(fmt.Sprintf("Registered %v activities", totalActivities))
	c.Log.Info(fmt.Sprintf("Output file '%v'", sheetName))

	file = xlsx.NewFile()
	sheet, err = file.AddSheet("Results")
	if err != nil {
		panic(err.Error())
	}

	row = sheet.AddRow()
	cell = row.AddCell()
	cell.Value = "Issue Id"
	cell = row.AddCell()
	cell.Value = "Issue Status"
	cell = row.AddCell()
	cell.Value = "Project Name"
	cell = row.AddCell()
	cell.Value = "Time to Resolve In Minutes"

	for _, activity := range activities {
		row = sheet.AddRow()
		cell = row.AddCell()
		cell.Value = activity.Issue.Id
		cell = row.AddCell()
		cell.Value = activity.Issue.Status
		cell = row.AddCell()
		cell.Value = activity.Issue.Project.Name
		cell = row.AddCell()
		cell.Value = strconv.FormatFloat(activity.Duration, 'f', 6, 64)
	}

	err = file.Save(sheetName)
	if err != nil {
		panic(err.Error())
	}
}

func (c *Calculator) calcMTTR(Issues []Issue) (mttr float64) {
	var totalIterations float64
	var totalTime float64

	totalIssues := len(Issues)

	c.Log.Info(fmt.Sprintf("Found %d issues", totalIssues))

	for _, issue := range Issues {
		c.Log.Info(fmt.Sprintf("Looking at issue #%v", issue.Id))

		if issue.Status == "unresolved" {
			c.Log.Info(fmt.Sprintf("Issue #%v dropped, unresolved", issue.Id))
		} else {
			auxTotalIterations, auxTotalTime := c.calcTimeToRepair(issue.Activity)

			activities = append(activities, ComputedActivity{Issue: issue, Duration: auxTotalTime})

			totalIterations += auxTotalIterations
			totalTime += auxTotalTime
		}
	}

	mttr = totalTime / totalIterations

	return
}

func (c *Calculator) calcTimeToRepair(activities []Activity) (totalIterations float64, totalTime float64) {
	c.Log.Info(fmt.Sprintf("Looking at %v activities", len(activities)))

	// We need to make it as reverse because of Sentry data
	for i := len(activities)-1; i >= 0; i-- {
		c.Log.Info(fmt.Sprintf("Activity #%s is '%s'", activities[i].Id, activities[i].Type))

		if activities[i].Type == "first_seen" {
			startTime, err := time.Parse(timeFormat, activities[i].DateCreated)
			if err != nil {
				panic(err)
			}

			i--

			if activities[i].Type == "set_resolved" {
				c.Log.Info(fmt.Sprintf("Activity #%s resolved in sequence", activities[i].Id))

				endTime, err := time.Parse(timeFormat, activities[i].DateCreated)
				if err != nil {
					panic(err)
				}

				duration := endTime.Sub(startTime).Minutes()

				totalIterations++
				totalTime += duration

				c.Log.Info(fmt.Sprintf("Took %.2f minutes to resolve", duration))
			}
		}
	}

	return totalIterations, totalTime
}

func (c *Calculator) requestProjects(cursor string) (resp *http.Response, err error) {
	client := &http.Client{}
	uri := fmt.Sprintf("%s0/projects/?query=&cursor=%s", sentryURL, cursor)

	c.Log.Debug(fmt.Sprintf("GET %s", uri))

	req, _ := http.NewRequest("GET", uri, nil)
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
	client := &http.Client{}
	uri := fmt.Sprintf("%s0/projects/%s/%s/issues/?query=&cursor=%s", sentryURL, project.Organization.Slug, project.Slug, cursor)

	c.Log.Debug(fmt.Sprintf("GET %s", uri))

	req, _ := http.NewRequest("GET", uri, nil)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", sentryToken))

	resp, err = client.Do(req)

	if err != nil {
		panic("Error while fetch data.")
	}

	return
}

func (c *Calculator) getIssues(project Project, cursor string) (issues []Issue) {
	resp, _ := c.requestIssues(project, cursor)
	currentIssues := []Issue{}

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
	client := &http.Client{}
	uri := fmt.Sprintf("%s0/issues/%s/", sentryURL, id)

	c.Log.Debug(fmt.Sprintf("GET %s", uri))

	req, _ := http.NewRequest("GET", uri, nil)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", sentryToken))

	resp, err = client.Do(req)

	if err != nil {
		panic("Error while fetch data.")
	}

	return
}
