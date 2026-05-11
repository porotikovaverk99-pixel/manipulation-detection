from contextlib import asynccontextmanager
from fastapi import FastAPI, HTTPException, BackgroundTasks
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, Field
from typing import Any, Dict, List, Optional
import uvicorn
import os

from advanced_analyzer import AdvancedManipulationAnalyzer
from db_integration import DatabaseConnector
from analyzer import ManipulationAnalyzer
from case_analyzer import CaseManipulationAnalyzer
from bot_analyzer import BotAccountAnalyzer
from trained_case_analyzer import TrainedCaseAnalyzer
from pheme_transformer_analyzer import PhemeTransformerAnalyzer

# Глобальные переменные для хранения ресурсов
db_connector = None
advanced_analyzer = None
simple_analyzer = None
case_analyzer = None
bot_analyzer = None
case_analyzer_mode = "unavailable"
pheme_transformer_analyzer = None
pheme_transformer_mode = "unavailable"
save_analysis_results = False

@asynccontextmanager
async def lifespan(app: FastAPI):
    # Startup: выполняется ПЕРЕД запуском приложения
    global db_connector, advanced_analyzer, simple_analyzer, case_analyzer, bot_analyzer, case_analyzer_mode, pheme_transformer_analyzer, pheme_transformer_mode, save_analysis_results
    
    print("🚀 Starting ML Service...")
    save_analysis_results = os.getenv("ML_SAVE_ANALYSIS_RESULTS", "false").lower() == "true"
    
    # Подключение к БД. Для /predict БД не обязательна, для /analyze/advanced нужна.
    conn_string = (
        os.getenv("DATABASE_URL")
        or os.getenv("DATABASE_URI")
        or "postgresql://postgres:password@localhost:5432/manipulation_detection"
    )
    try:
        db_connector = DatabaseConnector(conn_string)
        db_connector.connect()
        print("✅ Database connected")
    except Exception as exc:
        print(f"⚠️ Database unavailable, advanced DB signals degraded: {exc}")
        db_connector = None
    
    # Инициализация анализаторов
    advanced_analyzer = AdvancedManipulationAnalyzer(
        db_connection=db_connector.connection if db_connector else None,
        backend_url="http://localhost:8080"
    )
    simple_analyzer = ManipulationAnalyzer()
    bot_model_path = os.getenv("BOT_MODEL_PATH", "").strip()
    if bot_model_path:
        try:
            bot_analyzer = BotAccountAnalyzer(bot_model_path)
            print(f"✅ Bot analyzer loaded from {bot_model_path}")
        except Exception as exc:
            print(f"⚠️ Bot analyzer unavailable: {exc}")
            bot_analyzer = None
    else:
        bot_analyzer = None
    case_analyzer = CaseManipulationAnalyzer(bot_analyzer=bot_analyzer)
    case_analyzer_mode = "heuristic"
    case_model_path = os.getenv("CASE_MODEL_PATH", "").strip()
    if case_model_path:
        try:
            case_analyzer = TrainedCaseAnalyzer(case_model_path, bot_analyzer=bot_analyzer)
            case_analyzer_mode = "trained"
            print(f"✅ Trained case analyzer loaded from {case_model_path}")
        except Exception as exc:
            print(f"⚠️ Trained case analyzer unavailable, using heuristic fallback: {exc}")
            case_analyzer = CaseManipulationAnalyzer(bot_analyzer=bot_analyzer)
            case_analyzer_mode = "heuristic"

    pheme_transformer_path = os.getenv("PHEME_TRANSFORMER_MODEL_PATH", "").strip()
    if pheme_transformer_path:
        try:
            pheme_transformer_analyzer = PhemeTransformerAnalyzer(pheme_transformer_path)
            pheme_transformer_mode = "trained"
            print(f"✅ PHEME transformer analyzer loaded from {pheme_transformer_path}")
        except Exception as exc:
            print(f"⚠️ PHEME transformer analyzer unavailable: {exc}")
            pheme_transformer_analyzer = None
            pheme_transformer_mode = "unavailable"
    else:
        pheme_transformer_analyzer = None
        pheme_transformer_mode = "disabled"
    print("✅ Analyzers initialized")
    
    yield  # Здесь приложение работает и обрабатывает запросы
    
    # Shutdown: выполняется ПОСЛЕ остановки приложения
    print("🛑 Shutting down ML Service...")
    if db_connector and db_connector.connection:
        db_connector.connection.close()
        print("✅ Database connection closed")

# Создаем приложение с lifespan
app = FastAPI(
    title="Manipulation Detection API", 
    version="2.1.0",
    lifespan=lifespan  # <-- ВАЖНО: используем lifespan вместо on_event
)

# CORS настройки
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# ========== МОДЕЛИ ДАННЫХ ==========

class AnalyzeRequest(BaseModel):
    post_id: int
    content: str
    account_id: int
    username: str
    published_at: str
    followers_count: int
    account_created_at: Optional[str] = None

class BatchAnalyzeRequest(BaseModel):
    posts: List[AnalyzeRequest]

class SimplePredictRequest(BaseModel):
    text: str
    language: str = "ru"

class CasePostRequest(BaseModel):
    post_id: int
    external_id: str
    account_id: int
    username: str
    published_at: str
    content: str
    is_case_root: bool = False
    reply_to_post_id: Optional[int] = None
    likes_count: int = 0
    reposts_count: int = 0
    replies_count: int = 0
    followers_count: int = 0
    following_count: int = 0
    posts_count: int = 0
    is_verified: bool = False
    account_created_at: Optional[str] = None
    account_url: Optional[str] = None
    tags: List[str] = Field(default_factory=list)
    links: List[str] = Field(default_factory=list)

class CaseAnalyzeRequest(BaseModel):
    case_id: int
    external_case_id: str
    source_name: str
    dataset_name: Optional[str] = ""
    dataset_split: Optional[str] = ""
    case_type: str = "thread"
    label: Optional[str] = None
    title: Optional[str] = None
    event_name: Optional[str] = None
    first_event_at: Optional[str] = None
    last_event_at: Optional[str] = None
    posts: List[CasePostRequest]

class CaseAnalyzeResponse(BaseModel):
    feature_version: str
    score_version: str
    pipeline_hash: str
    risk_score: float
    risk_level: str
    confidence_score: float
    temporal_score: float
    coordination_score: float
    content_score: float
    event_count: int
    unique_account_count: int
    unique_url_count: int
    unique_hashtag_count: int
    temporal_features: Dict[str, Any]
    coordination_features: Dict[str, Any]
    content_features: Dict[str, Any]
    feature_payload: Dict[str, Any]
    evidence: List[str]
    model_info: Dict[str, Any]

class CaseTextAnalyzeResponse(BaseModel):
    model_version: str
    model_path: str
    pipeline_hash: str
    risk_score: float
    risk_level: str
    confidence_score: float
    text_mode: str
    max_length: int
    reaction_count_used: int
    feature_payload: Dict[str, Any]
    evidence: List[str]
    model_info: Dict[str, Any]

class BotTweetRequest(BaseModel):
    text: str = ""
    source: Optional[str] = None
    in_reply_to_status_id: Optional[str] = None
    retweeted_status_id: Optional[str] = None
    retweet_count: float = 0
    reply_count: float = 0
    favorite_count: float = 0
    num_hashtags: float = 0
    num_urls: float = 0
    num_mentions: float = 0

class BotAnalyzeRequest(BaseModel):
    account_id: Optional[str] = None
    username: Optional[str] = None
    statuses_count: float = 0
    followers_count: float = 0
    friends_count: float = 0
    favourites_count: float = 0
    listed_count: float = 0
    default_profile: bool = False
    default_profile_image: bool = False
    geo_enabled: bool = False
    verified: bool = False
    protected: bool = False
    description: Optional[str] = None
    url: Optional[str] = None
    location: Optional[str] = None
    created_at: Optional[str] = None
    updated_at: Optional[str] = None
    tweets: List[BotTweetRequest] = Field(default_factory=list)

class BotAnalyzeResponse(BaseModel):
    model_version: str
    model_path: str
    bot_score: float
    predicted_label: str
    feature_payload: Dict[str, Any]
    top_contributors: List[Dict[str, Any]]

# ========== ЭНДПОИНТЫ ==========

@app.get("/health")
async def health():
    return {
        "status": "ok", 
        "service": "ml-detector", 
        "version": "2.1",
        "analyzers": {
            "advanced": advanced_analyzer is not None,
            "simple": simple_analyzer is not None,
            "case": case_analyzer is not None,
            "bot": bot_analyzer is not None,
            "pheme_transformer": pheme_transformer_analyzer is not None
        },
        "case_mode": case_analyzer_mode,
        "pheme_transformer_mode": pheme_transformer_mode,
    }

@app.post("/predict")
async def simple_predict(request: SimplePredictRequest):
    """Простой эндпоинт для обратной совместимости с collector"""
    if not simple_analyzer:
        raise HTTPException(status_code=503, detail="Simple analyzer not initialized")
    
    result = simple_analyzer.analyze_text(request.text)
    
    return {
        "manipulation_score": result["manipulation_score"],
        "confidence_score": result["confidence_score"],
        "coordination_contribution": result.get("coordination_contribution", 0),
        "temporal_contribution": result.get("temporal_contribution", 0),
        "narrative_contribution": result.get("narrative_contribution", 0),
        "confidence_note": result["confidence_note"],
        "key_evidence": result["key_evidence"],
        "tactics": result["tactics"]
    }

@app.post("/analyze/advanced")
async def analyze_advanced(request: AnalyzeRequest, background_tasks: BackgroundTasks):
    """Расширенный анализ с учетом контекста, координации и поведения аккаунта"""
    if not advanced_analyzer:
        raise HTTPException(status_code=503, detail="Advanced analyzer not initialized")
    
    try:
        from datetime import datetime
        published_at = datetime.fromisoformat(request.published_at.replace('Z', '+00:00'))
        account_created_at = None
        if request.account_created_at:
            account_created_at = datetime.fromisoformat(request.account_created_at.replace('Z', '+00:00'))
        
        # Комплексный анализ
        result = await advanced_analyzer.comprehensive_analysis(
            post_id=request.post_id,
            content=request.content,
            account_id=request.account_id,
            username=request.username,
            published_at=published_at,
            followers_count=request.followers_count,
            account_created_at=account_created_at or datetime.now()
        )
        
        # В dataset-first pipeline результат сохраняет backend dataset_analyzer.
        if db_connector and save_analysis_results:
            background_tasks.add_task(
                db_connector.save_analysis_result,
                request.post_id,
                result
            )
        
        return result
        
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/analyze/batch")
async def analyze_batch(request: BatchAnalyzeRequest):
    """Пакетный анализ постов"""
    if not advanced_analyzer:
        raise HTTPException(status_code=503, detail="Advanced analyzer not initialized")
    
    results = []
    for post in request.posts:
        try:
            from datetime import datetime
            published_at = datetime.fromisoformat(post.published_at.replace('Z', '+00:00'))
            account_created_at = None
            if post.account_created_at:
                account_created_at = datetime.fromisoformat(post.account_created_at.replace('Z', '+00:00'))
            
            result = await advanced_analyzer.comprehensive_analysis(
                post_id=post.post_id,
                content=post.content,
                account_id=post.account_id,
                username=post.username,
                published_at=published_at,
                followers_count=post.followers_count,
                account_created_at=account_created_at or datetime.now()
            )
            results.append(result)
        except Exception as e:
            results.append({"error": str(e), "post_id": post.post_id})
    
    return {"results": results}

@app.post("/analyze/case", response_model=CaseAnalyzeResponse)
async def analyze_case(request: CaseAnalyzeRequest):
    """Case-level baseline analysis for threads/clusters."""
    if not case_analyzer:
        raise HTTPException(status_code=503, detail="Case analyzer not initialized")

    try:
        payload = request.model_dump()
        return case_analyzer.analyze_case(payload)
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/analyze/case-text", response_model=CaseTextAnalyzeResponse)
async def analyze_case_text(request: CaseAnalyzeRequest):
    """Case-level transformer analysis for PHEME-style source tweet + reactions."""
    if not pheme_transformer_analyzer:
        raise HTTPException(status_code=503, detail="PHEME transformer analyzer not initialized")

    try:
        payload = request.model_dump()
        return pheme_transformer_analyzer.analyze_case(payload)
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@app.post("/analyze/bot-account", response_model=BotAnalyzeResponse)
async def analyze_bot_account(request: BotAnalyzeRequest):
    """Account-level bot likelihood based on Cresci-trained baseline."""
    if not bot_analyzer:
        raise HTTPException(status_code=503, detail="Bot analyzer not initialized")

    try:
        payload = request.model_dump()
        return bot_analyzer.analyze_account(payload)
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

@app.get("/account/{account_id}/analysis")
async def analyze_account(account_id: int):
    """Анализ всех постов аккаунта"""
    if not advanced_analyzer or not db_connector:
        raise HTTPException(status_code=503, detail="Service not fully initialized")
    
    posts = db_connector.get_account_posts(account_id, limit=50)
    
    if not posts:
        raise HTTPException(status_code=404, detail="Account not found or no posts")
    
    account_info = db_connector.get_account_info(account_id)
    
    results = []
    for post in posts:
        result = await advanced_analyzer.comprehensive_analysis(
            post_id=post['id'],
            content=post['content'],
            account_id=account_id,
            username=account_info['username'] if account_info else "unknown",
            published_at=post['published_at'],
            followers_count=account_info['followers_count'] if account_info else 0,
            account_created_at=account_info['created_at'] if account_info else None
        )
        results.append(result)
    
    avg_score = sum(r['manipulation_score'] for r in results) / len(results) if results else 0
    high_manipulation_count = sum(1 for r in results if r['manipulation_score'] > 0.6)
    
    return {
        "account_id": account_id,
        "username": account_info['username'] if account_info else "unknown",
        "total_posts_analyzed": len(results),
        "average_manipulation_score": round(avg_score, 3),
        "high_manipulation_posts": high_manipulation_count,
        "results": results
    }

if __name__ == "__main__":
    uvicorn.run(
        "main:app",  # <-- ВАЖНО: передаем import string вместо объекта
        host="0.0.0.0",
        port=8000,
        reload=True  # reload работает только с import string
    )
