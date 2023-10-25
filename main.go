package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/pkg/errors"
)

type Workflows struct {
	Message struct {
		Keys   []string   `json:"keys"`
		Values [][]string `json:"values"`
	} `json:"message"`
}

type Workflow struct {
	Docs []struct {
		Name   string `json:"name"`
		States []struct {
			State     string `json:"state"`
			AllowEdit string `json:"allow_edit"`
		} `json:"states"`
		Transitions []struct {
			State     string `json:"state"`
			Action    string `json:"action"`
			NextState string `json:"next_state"`
			Allowed   string `json:"allowed"`
		}
	} `json:"docs"`
}

type Transition struct {
	From   string
	To     string
	Action string
}

var baseURL string
var auth string

func init() {
	err := godotenv.Load(".env")

	if err != nil {
		log.Fatal("Error loading .env file")
	}

	baseURL = os.Getenv("FRAPPE_BASE_URL")
	apiKey := os.Getenv("FRAPPE_API_KEY")
	apiSecret := os.Getenv("FRAPPE_API_SECRET")

	auth = fmt.Sprintf("token %s:%s", apiKey, apiSecret)
}

func main() {
	workflows := getWorkflows()

	var wfMap = make(map[int]string)

	fmt.Println("Please select a workflow:")
	for k, v := range workflows {
		wfMap[k] = v
		fmt.Printf("[%d] - %s\n", k, v)
	}

	selectedWF := selectWorkflow(wfMap)
	workflow := getWorkflow(selectedWF)

	var roleStateMap = map[string][]string{}
	var roleTransitionMap = map[string][]Transition{}

	for _, w := range workflow.Docs {

		states := w.States

		for _, s := range states {
			role := s.AllowEdit
			state := s.State

			roleStateMap[role] = append(roleStateMap[role], state)
		}

		transitions := w.Transitions

		for _, t := range transitions {
			role := t.Allowed
			from := t.State
			to := t.NextState
			action := t.Action

			roleTransitionMap[role] = append(roleTransitionMap[role], Transition{From: from, To: to, Action: action})
		}
	}

	diagram := createDiagram(roleStateMap, roleTransitionMap)
	encoded, err := encodeKroki(diagram)
	if err != nil {
		log.Fatal("Cannot generate diagram URL", err)
	}

	url := fmt.Sprintf("https://kroki.io/actdiag/svg/%s", encoded)
	fmt.Printf("Please click to the following URL:\n%s\n", url)

}

func createDiagram(stateMap map[string][]string, transMap map[string][]Transition) string {

	var str strings.Builder

	str.WriteString("actdiag {\n")
	for k, stat := range stateMap {
		lane := fmt.Sprintf("lane \"%s\" {\n", k)
		str.WriteString(lane)

		for _, s := range stat {
			statLine := fmt.Sprintf("\t\"%s\"\n", s)
			str.WriteString(statLine)
		}

		str.WriteString("}\n")
	}

	for _, trans := range transMap {
		for _, t := range trans {
			transLine := fmt.Sprintf("\t\"%s\" -> \"%s\" [label = \"%s\"]\n", t.From, t.To, t.Action)
			str.WriteString(transLine)
		}
	}
	str.WriteString("}")
	return str.String()
}

func selectWorkflow(wfMap map[int]string) string {
	var input int
	var ret string

	fmt.Scanf("%d \n", &input)

	if wfMap[input] == "" {
		fmt.Println("Please enter a valid number")
		selectWorkflow(wfMap)
	} else {
		ret = wfMap[input]
	}
	return ret
}

func getWorkflows() []string {
	url := fmt.Sprintf("%s/api/method/frappe.desk.reportview.get", baseURL)

	payload := strings.NewReader("-----011000010111000001101001\r\nContent-Disposition: form-data; name=\"doctype\"\r\n\r\nWorkflow\r\n-----011000010111000001101001--\r\n")
	req, _ := http.NewRequest("POST", url, payload)

	req.Header.Add("Authorization", auth)
	req.Header.Add("content-type", "multipart/form-data; boundary=---011000010111000001101001")

	res, err := http.DefaultClient.Do(req)

	if err != nil {
		log.Fatal(err)
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}

	var wf Workflows

	err = json.Unmarshal(body, &wf)
	if err != nil {
		log.Fatal(err)
	}

	var ret []string
	for _, v := range wf.Message.Values {
		ret = append(ret, strings.Join(v, ""))
	}

	return ret
}

func getWorkflow(name string) Workflow {

	params := url.Values{}
	params.Add("doctype", "Workflow")
	params.Add("name", name)

	url := fmt.Sprintf("%s/api/method/frappe.desk.form.load.getdoc?%s", baseURL, params.Encode())

	req, _ := http.NewRequest("GET", url, nil)

	req.Header.Add("Accept", "*/*")
	req.Header.Add("Authorization", auth)

	res, err := http.DefaultClient.Do(req)

	if err != nil {
		log.Fatal(err)
	}

	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)

	if err != nil {
		log.Fatal(err)
	}

	var wf Workflow

	err = json.Unmarshal(body, &wf)
	if err != nil {
		log.Fatal(err)
	}

	return wf
}

func encodeKroki(input string) (string, error) {
	var buffer bytes.Buffer
	writer, err := zlib.NewWriterLevel(&buffer, 9)
	if err != nil {
		return "", errors.Wrap(err, "fail to create the writer")
	}
	_, err = writer.Write([]byte(input))
	writer.Close()
	if err != nil {
		return "", errors.Wrap(err, "fail to create the payload")
	}
	result := base64.URLEncoding.EncodeToString(buffer.Bytes())
	return result, nil
}
