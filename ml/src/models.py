from pydantic import BaseModel
from typing import List, Dict, Optional

class TextRequest(BaseModel):
    text: str
    language: str = "ru"

class ManipulationResponse(BaseModel):
    manipulation_score: float
    confidence_score: float
    coordination_contribution: float
    temporal_contribution: float
    narrative_contribution: float
    confidence_note: str
    key_evidence: List[str]
    tactics: List[str]

class BatchRequest(BaseModel):
    texts: List[TextRequest]