package main

import (
	"encoding/json"
)

type CodeAnalysisResult struct {
	QualityScore  int      `json:"quality_score"`
	Bugs          []string `json:"bugs"`
	Optimizations []string `json:"optimizations"`
	Security      []string `json:"security"`
}

type DataProcessingResult struct {
	Result   interface{} `json:"result"`
	Summary  string      `json:"summary"`
	Insights []string    `json:"insights"`
}

type Task struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Priority     string   `json:"priority"`
	Effort       int      `json:"effort"`
	Dependencies []string `json:"dependencies"`
}

func AnalyzeCode(code, language string) (CodeAnalysisResult, error) {
	prompt := "Analyze this " + language + " code and provide a JSON response with: 1. Code quality assessment (1-10 score) 2. Potential bugs or issues (list) 3. Optimization suggestions (list) 4. Security concerns (list)\n\nCode:\n" + code + "\nRespond only with valid JSON."
	resp, err := GenerateLLMResponse(prompt)
	if err != nil {
		return CodeAnalysisResult{}, err
	}
	var result CodeAnalysisResult
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return CodeAnalysisResult{}, err
	}
	return result, nil
}

func ProcessData(data map[string]interface{}, task string) (DataProcessingResult, error) {
	dataJson, _ := json.Marshal(data)
	prompt := "Process this data according to the task: " + task + "\n\nData: " + string(dataJson) + "\nProvide a structured response with: 1. Processed result 2. Summary of changes 3. Any insights or patterns noticed\nRespond in JSON format."
	resp, err := GenerateLLMResponse(prompt)
	if err != nil {
		return DataProcessingResult{}, err
	}
	var result DataProcessingResult
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		return DataProcessingResult{}, err
	}
	return result, nil
}

func PlanTasks(requirements string) ([]Task, error) {
	prompt := "Break down these requirements into specific tasks:\n\nRequirements: " + requirements + "\nCreate a task list with: - Task name - Description - Priority (high/medium/low) - Estimated effort (hours) - Dependencies\nRespond as a JSON array of task objects."
	resp, err := GenerateLLMResponse(prompt)
	if err != nil {
		return nil, err
	}
	var tasks []Task
	if err := json.Unmarshal([]byte(resp), &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func AnswerQuestion(question, context string) (string, error) {
	prompt := question
	if context != "" {
		prompt = "Context: " + context + "\n\nQuestion: " + question
	}
	return GenerateLLMResponse(prompt)
}
