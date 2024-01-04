package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/google/go-github/v57/github"
)

var (
	endpoint = os.Getenv("AZ_AI_ENDPOINT")
	key      = os.Getenv("AZ_AI_KEY")

	owner = os.Getenv("GITHUB_OWNER")
	repo  = os.Getenv("GITHUB_REPO")

	confidence = os.Getenv("CONFIDENCE_SCORE_THRESHOLD")
)

type PiiEntityRecognitionRequest struct {
	Kind          string        `json:"kind"`
	AnalysisInput AnalysisInput `json:"analysisInput"`
	Parameters    Parameters    `json:"parameters"`
}

type AnalysisInput struct {
	Documents []Document `json:"documents"`
}

type Document struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Language string `json:"language"`
}

type Parameters struct {
	Domain string `json:"domain"`
}

type PiiEntityRecognitionResults struct {
	Kind    string  `json:"kind"`
	Results Results `json:"results"`
}

type Results struct {
	Documents    []DocumentResponse `json:"documents"`
	Errors       []Error            `json:"errors"`
	ModelVersion string             `json:"modelVersion"`
}

type DocumentResponse struct {
	RedactedText string    `json:"redactedText"`
	ID           string    `json:"id"`
	Entities     []Entity  `json:"entities"`
	Warnings     []Warning `json:"warnings"`
}

type Entity struct {
	Text            string  `json:"text"`
	Category        string  `json:"category"`
	Offset          int     `json:"offset"`
	Length          int     `json:"length"`
	ConfidenceScore float64 `json:"confidenceScore"`
}

type Warning struct{}

type Error struct{}

func main() {
	page := 0
	client := &http.Client{}
	githubClient := github.NewClient(nil).WithAuthToken(os.Getenv("GITHUB_PAT"))

	confidenceThreshold, err := strconv.ParseFloat(confidence, 64)
	if err != nil {
		fmt.Println("Error parsing confidence score threshold:", err)
		return
	}
	for {
		issues, res, err := githubClient.Issues.ListByRepo(context.Background(), owner, repo, &github.IssueListByRepoOptions{
			State: "all",
			ListOptions: github.ListOptions{
				PerPage: 5,
				Page:    page,
			},
		})
		if err != nil {
			fmt.Println("Error getting issues:", err)
			return
		}
		page++

		var documents []Document
		for _, issue := range issues {
			documents = append(documents, Document{
				Language: "en",
				ID:       issue.GetHTMLURL(),
				Text:     issue.GetBody(),
			})
		}

		requestData := PiiEntityRecognitionRequest{
			Kind: "PiiEntityRecognition",
			AnalysisInput: AnalysisInput{
				Documents: documents,
			},
			Parameters: Parameters{
				Domain: "phi",
			},
		}

		requestBytes, err := json.Marshal(requestData)
		if err != nil {
			fmt.Println("Error marshalling request data:", err)
			return
		}

		serviceEndpoint := fmt.Sprintf("%s/language/:analyze-text?api-version=2022-05-01", endpoint)
		req, err := http.NewRequest("POST", serviceEndpoint, bytes.NewBuffer(requestBytes))
		if err != nil {
			fmt.Println("Error creating request:", err)
			return
		}

		req.Header.Add("Ocp-Apim-Subscription-Key", key)
		req.Header.Add("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			fmt.Println("Error executing request:", err)
			return
		}
		defer resp.Body.Close()

		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Println("Error reading response body:", err)
			return
		}

		var textResponse PiiEntityRecognitionResults
		err = json.Unmarshal(responseBody, &textResponse)
		if err != nil {
			fmt.Println("Error unmarshalling response body:", err)
			return
		}

		for _, doc := range textResponse.Results.Documents {
			var entities []Entity
			for _, entity := range doc.Entities {
				if entity.ConfidenceScore >= confidenceThreshold {
					entities = append(entities, entity)
				}
			}
			if len(entities) > 0 {
				result := struct {
					ID       string   `json:"id"`
					Redacted string   `json:"redacted"`
					Entities []Entity `json:"entities"`
				}{
					ID:       doc.ID,
					Redacted: doc.RedactedText,
					Entities: entities,
				}
				resultBytes, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					fmt.Println("Error marshalling result:", err)
					return
				}
				fmt.Println(string(resultBytes))
			}
		}

		if res.NextPage == 0 {
			break
		}
	}
}
