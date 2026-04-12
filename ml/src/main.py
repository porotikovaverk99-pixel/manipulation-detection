from contextlib import asynccontextmanager
from fastapi import FastAPI, HTTPException, BackgroundTasks
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
from typing import List, Optional
import uvicorn

from advanced_analyzer import AdvancedManipulationAnalyzer
from db_integration import DatabaseConnector
from analyzer import ManipulationAnalyzer

# Глобальные переменные для хранения ресурсов
db_connector = None
advanced_analyzer = None
simple_analyzer = None

@asynccontextmanager
async def lifespan(app: FastAPI):
    # Startup: выполняется ПЕРЕД запуском приложения
    global db_connector, advanced_analyzer, simple_analyzer
    
    print("🚀 Starting ML Service...")
    
    # Подключение к БД
    conn_string = "postgresql://postgres:123@localhost:5432/manipulation_detection"
    db_connector = DatabaseConnector(conn_string)
    db_connector.connect()
    print("✅ Database connected")
    
    # Инициализация анализаторов
    advanced_analyzer = AdvancedManipulationAnalyzer(
        db_connection=db_connector.connection,
        backend_url="http://localhost:8080"
    )
    simple_analyzer = ManipulationAnalyzer()
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
    version="2.0.0",
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

# ========== ЭНДПОИНТЫ ==========

@app.get("/health")
async def health():
    return {
        "status": "ok", 
        "service": "ml-detector", 
        "version": "2.0",
        "analyzers": {
            "advanced": advanced_analyzer is not None,
            "simple": simple_analyzer is not None
        }
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
        
        # Сохранение результатов в фоне
        if db_connector:
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