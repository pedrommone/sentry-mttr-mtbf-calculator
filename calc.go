package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/bradfitz/slice"
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
	Name		string `json:"name"`
	Slug		string `json:"slug"`
	Organization	Organization
}

type Issue struct {
	Id		string `json:"id"`
	Status		string `json:"status"`
	Project		Project
	Activity		[]Activity
}

type Activity struct {
	Id		string `json:"id"`
	DateCreated	string `json:"dateCreated"`
 	Type		string `json:"type"`
}

type Event struct {
	Id		string `json:"eventID"`
	DateCreated	string `json:"dateCreated"`
}

type ComputedEvent struct {
	Event		Event
	Duration	float64
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
	activities	[]ComputedActivity
	events		[]Event
	eventsMTBF	[]ComputedEvent
	issues		[]Issue
	projects	[]Project
	sentryToken	string
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
	// Hack for keep things fast.
	// projects = []Project{Project{Name: "arya", Slug: "arya", Organization: Organization{Slug: "ezdelivery"}}}

	for _, project := range projects {
		issues = append(issues, c.getIssues(project, "0:0:0")...)
	}

	for _, issue := range issues {
		events = append(events, c.getEvents(issue, "0:0:0")...)
	}

	c.sortEventsBasedOnTime()

	c.Log.Debug("====================")
	c.Log.Debug("Dataset")
	c.Log.Debug(fmt.Sprintf("%# v", pretty.Formatter(issues)))
	c.Log.Debug("====================")
	c.Log.Debug(fmt.Sprintf("%# v", pretty.Formatter(events)))
	c.Log.Debug("====================")

	mttr := c.calcMTTR(issues)
	c.Log.Info(fmt.Sprintf("MTTR: %.0f seconds", mttr))

	mtbf := c.calcMTBF(events)
	c.Log.Info(fmt.Sprintf("MTBF: %.0f seconds", mtbf))

	c.saveActivitiesIntoXLSX(activities)
}

func (c *Calculator) sortEventsBasedOnTime() {
	slice.Sort(events[:], func(i, j int) bool {
		return events[i].DateCreated < events[j].DateCreated
	})
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
	sheet, err = file.AddSheet("MTTR")
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
	cell.Value = "Time to Resolve In Seconds"

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

func (c *Calculator) calcMTBF(events []Event) (mtbf float64) {
	lastTime := ""

	for _, event := range events {
		if lastTime != "" {
			lastEventDate, err := time.Parse(timeFormat, lastTime)
			if err != nil {
				panic(err)
			}

			currentEventDate, err := time.Parse(timeFormat, event.DateCreated)
			if err != nil {
				panic(err)
			}

			duration := currentEventDate.Sub(lastEventDate).Seconds()
			eventsMTBF = append(eventsMTBF, ComputedEvent{Event: event, Duration: duration})

			c.Log.Debug(fmt.Sprintf("Event #%v took %.0f seconds to appear", event.Id, duration))
		} else {
			c.Log.Debug(fmt.Sprintf("Event #%v is new, not computed", event.Id))
		}

		lastTime = event.DateCreated
	}

	totalIterations, totalTime := c.calcMediumTimeForMTTR()
	mtbf = totalTime / totalIterations

	return
}

func (c *Calculator) calcMediumTimeForMTTR() (totalIterations float64, totalTime float64) {
	totalIterations = 0
	totalTime = 0

	for _, event := range eventsMTBF {
		totalIterations++
		totalTime += event.Duration
	}

	return
}

func (c *Calculator) calcMTTR(issues []Issue) (mttr float64) {
	var totalIterations float64
	var totalTime float64

	totalIssues := len(issues)

	c.Log.Debug(fmt.Sprintf("Found %d issues", totalIssues))

	for _, issue := range issues {
		c.Log.Debug(fmt.Sprintf("Looking at issue #%v", issue.Id))

		if issue.Status == "unresolved" {
			c.Log.Debug(fmt.Sprintf("Issue #%v dropped, unresolved", issue.Id))
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
	c.Log.Debug(fmt.Sprintf("Looking at %v activities", len(activities)))

	// We need to make it as reverse because of Sentry data
	for i := len(activities)-1; i >= 0; i-- {
		c.Log.Debug(fmt.Sprintf("Activity #%s is '%s'", activities[i].Id, activities[i].Type))

		if activities[i].Type == "first_seen" {
			startTime, err := time.Parse(timeFormat, activities[i].DateCreated)
			if err != nil {
				panic(err)
			}

			i--

			if activities[i].Type == "set_resolved" {
				c.Log.Debug(fmt.Sprintf("Activity #%s resolved in sequence", activities[i].Id))

				endTime, err := time.Parse(timeFormat, activities[i].DateCreated)
				if err != nil {
					panic(err)
				}

				duration := endTime.Sub(startTime).Seconds()

				totalIterations++
				totalTime += duration

				c.Log.Debug(fmt.Sprintf("Took %.0f seconds to resolve", duration))
			}
		}
	}

	return totalIterations, totalTime
}

func (c *Calculator) requestEvents(issue Issue, cursor string) (resp *http.Response, err error) {
	client := &http.Client{}
	uri := fmt.Sprintf("%s0/issues/%s/events/?query=&cursor=%s", sentryURL, issue.Id, cursor)

	c.Log.Debug(fmt.Sprintf("GET %s", uri))

	req, _ := http.NewRequest("GET", uri, nil)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", sentryToken))

	resp, err = client.Do(req)

	if err != nil {
		panic("Error while fetch data.")
	}

	return
}

func (c *Calculator) getEvents(issue Issue, cursor string) (events []Event) {
	resp, _ := c.requestEvents(issue, cursor)

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	err = json.Unmarshal(b, &events)
	if err != nil {
		panic(err)
	}

	link := resp.Header.Get("Link")
	links := linkheader.Parse(link)
	nextPage := links[1].Params

	if nextPage["results"] == "true" {
		c.getEvents(issue, nextPage["cursor"])
	}

	return
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
