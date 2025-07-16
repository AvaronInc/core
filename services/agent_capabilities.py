from typing import Dict, Any, List
import json
import logging
from .llm_service import LLMService

logger = logging.getLogger(__name__)

class AgentCapabilities:
    def __init__(self, llm_service: LLMService):
        self.llm = llm_service
        self.capabilities = {
            "code_analysis": self.analyze_code,
            "data_processing": self.process_data,
            "task_planning": self.plan_tasks,
            "question_answering": self.answer_question
        }
    
    async def analyze_code(self, code: str, language: str) -> Dict[str, Any]:
        """Analyze code and provide insights"""
        prompt = f"""Analyze this {language} code and provide a JSON response with:
        1. Code quality assessment (1-10 score)
        2. Potential bugs or issues (list)
        3. Optimization suggestions (list)
        4. Security concerns (list)
        
        Code:
        ```{language}
        {code}
        ```
        
        Respond only with valid JSON."""
        
        try:
            response = await self.llm.generate_response(prompt)
            # Clean response to ensure valid JSON
            response = response.strip()
            if response.startswith("```json"):
                response = response[7:]
            if response.endswith("```"):
                response = response[:-3]
            return json.loads(response)
        except json.JSONDecodeError:
            logger.error(f"Failed to parse JSON response: {response}")
            return {
                "error": "Failed to parse response",
                "raw_response": response
            }
    
    async def process_data(self, data: Dict, task: str) -> Dict[str, Any]:
        """Process data according to specified task"""
        prompt = f"""Process this data according to the task: {task}
        
        Data: {json.dumps(data)}
        
        Provide a structured response with:
        1. Processed result
        2. Summary of changes
        3. Any insights or patterns noticed
        
        Respond in JSON format."""
        
        response = await self.llm.generate_response(prompt)
        try:
            return json.loads(response)
        except:
            return {"result": response}
    
    async def plan_tasks(self, requirements: str) -> List[Dict]:
        """Break down requirements into actionable tasks"""
        prompt = f"""Break down these requirements into specific tasks:
        
        Requirements: {requirements}
        
        Create a task list with:
        - Task name
        - Description
        - Priority (high/medium/low)
        - Estimated effort (hours)
        - Dependencies
        
        Respond as a JSON array of task objects."""
        
        response = await self.llm.generate_response(prompt)
        try:
            return json.loads(response)
        except:
            return [{"error": "Failed to parse tasks", "raw": response}]
    
    async def answer_question(self, question: str, context: str = "") -> str:
        """Answer a general question"""
        prompt = question
        if context:
            prompt = f"Context: {context}\n\nQuestion: {question}"
        
        return await self.llm.generate_response(prompt) 