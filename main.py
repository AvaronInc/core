from fastapi import FastAPI, HTTPException, Response
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from typing import List, Dict, Optional
from prometheus_client import Counter, Histogram, generate_latest
import time
import logging
from services.llm_service import LLMService

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Create FastAPI app
app = FastAPI(
    title="Avaron AI Agent",
    description="Open-source AI agent for edge deployment",
    version="1.0.0"
)

# Add CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Metrics
request_count = Counter('agent_requests_total', 'Total requests to the agent')
request_duration = Histogram('agent_request_duration_seconds', 'Request duration in seconds')
error_count = Counter('agent_errors_total', 'Total errors')

# Initialize LLM service
llm_service = LLMService()

# Request/Response models
class AgentRequest(BaseModel):
    prompt: str
    context: Optional[List[Dict]] = []
    temperature: Optional[float] = 0.7
    max_tokens: Optional[int] = 1000

class AgentResponse(BaseModel):
    response: str
    model: str
    processing_time: float
    tokens_used: Optional[int] = None

class HealthResponse(BaseModel):
    status: str
    model: str
    version: str

# Health check endpoint
@app.get("/health", response_model=HealthResponse)
async def health_check():
    """Check if the service is healthy"""
    return HealthResponse(
        status="healthy",
        model="mistral:7b",
        version="1.0.0"
    )

# Main agent query endpoint
@app.post("/api/v1/agent/query", response_model=AgentResponse)
async def query_agent(request: AgentRequest):
    """Query the AI agent with a prompt"""
    start_time = time.time()
    request_count.inc()
    
    try:
        logger.info(f"Processing query: {request.prompt[:100]}...")
        
        response = await llm_service.generate_response(
            prompt=request.prompt,
            context=request.context
        )
        
        processing_time = time.time() - start_time
        request_duration.observe(processing_time)
        
        return AgentResponse(
            response=response,
            model="mistral:7b",
            processing_time=processing_time
        )
    except Exception as e:
        error_count.inc()
        logger.error(f"Error processing request: {e}")
        raise HTTPException(status_code=500, detail=str(e))

# Metrics endpoint
@app.get("/metrics")
async def metrics():
    """Prometheus metrics endpoint"""
    return Response(content=generate_latest(), media_type="text/plain")

# Root endpoint
@app.get("/")
async def root():
    """Root endpoint with basic info"""
    return {
        "service": "Avaron AI Agent",
        "version": "1.0.0",
        "endpoints": {
            "health": "/health",
            "query": "/api/v1/agent/query",
            "metrics": "/metrics"
        }
    }

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8000) 