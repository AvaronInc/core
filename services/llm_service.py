import ollama
from typing import Dict, List
import asyncio
import logging

logger = logging.getLogger(__name__)

class LLMService:
    def __init__(self, model_name: str = "mistral:7b"):
        self.client = ollama.Client()
        self.model = model_name
        
    async def generate_response(self, prompt: str, context: List[Dict] = None) -> str:
        """Generate response from LLM"""
        try:
            messages = context or []
            messages.append({"role": "user", "content": prompt})
            
            # Run in executor to avoid blocking
            loop = asyncio.get_event_loop()
            response = await loop.run_in_executor(
                None, 
                lambda: self.client.chat(
                    model=self.model,
                    messages=messages,
                    stream=False
                )
            )
            return response['message']['content']
        except Exception as e:
            logger.error(f"LLM generation error: {e}")
            raise 