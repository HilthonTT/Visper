from pydantic import BaseModel

class HealthCheckResponse(BaseModel):
    status: str
    environment: str
    version: str
    timestamp: str
    
class ReadyCheckResponse(BaseModel):
    status: str
    environment: str
    version: str
    app: str
    redis: str
    timestamp: str
