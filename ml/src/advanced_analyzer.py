import re
import numpy as np
from datetime import datetime, timedelta, timezone
from typing import List, Dict, Tuple, Optional
from collections import Counter, defaultdict
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.metrics.pairwise import cosine_similarity
from sklearn.ensemble import RandomForestClassifier
import hashlib
import json
import os
import requests
from dataclasses import dataclass
import asyncio
import aiohttp
import traceback
import sys

@dataclass
class PostContext:
    """Контекст поста из БД"""
    post_id: int
    content: str
    account_id: int
    username: str
    published_at: datetime
    followers_count: int
    account_created_at: datetime
    account_posts_count: int
    likes_count: int
    reposts_count: int
    replies_count: int

class AdvancedManipulationAnalyzer:
    def __init__(self, db_connection=None, backend_url="http://localhost:8080"):
        """
        db_connection: подключение к PostgreSQL для получения данных
        backend_url: URL бэкенда для API запросов
        """
        print("🔧 Инициализация AdvancedManipulationAnalyzer...")
        self.db = db_connection
        self.backend_url = backend_url
        
        print(f"   - db_connection: {'есть' if db_connection else 'НЕТ'}")
        print(f"   - backend_url: {backend_url}")
        self.enable_live_comment_fetch = os.getenv("ENABLE_LIVE_COMMENT_FETCH", "false").lower() == "true"
        
        # Загрузка ML модели (будет обучена на реальных данных)
        self.classifier = None
        self.vectorizer = TfidfVectorizer(max_features=1000, ngram_range=(1, 3))
        
        # Кэш для данных аккаунтов
        self.account_cache = {}
        
        # Словари манипулятивных маркеров (расширенные)
        self.manipulation_patterns = self._load_manipulation_patterns()
        print("✅ AdvancedManipulationAnalyzer инициализирован")
        
    def _load_manipulation_patterns(self) -> Dict:
        """Загрузка паттернов манипуляций"""
        return {
            # Эмоциональная манипуляция
            'emotional': {
                'fear': ['ужас', 'кошмар', 'катастрофа', 'апокалипсис', 'гибель', 'смерть', 
                        'terror', 'horror', 'nightmare', 'catastrophe', 'apocalypse'],
                'anger': ['возмущение', 'гнев', 'ярость', 'предательство', 'обман', 
                         'outrage', 'anger', 'betrayal', 'scandal'],
                'urgency': ['срочно', 'немедленно', 'прямо сейчас', 'urgent', 'immediately', 
                           'breaking', 'alert']
            },
            # Логические ошибки
            'fallacies': {
                'ad_populum': ['все так считают', 'большинство', 'каждый', 'everyone', 'nobody'],
                'false_dilemma': ['или мы или они', 'выбор без выбора', 'either or', 'only choice'],
                'conspiracy': ['скрывают правду', 'заговор', 'секретно', 'conspiracy', 'cover up']
            },
            # Координационные маркеры
            'coordination': {
                'share': ['репост', 'ретвит', 'share', 'retweet', 'распространи'],
                'action': ['подпишись', 'лайкни', 'напиши', 'follow', 'like', 'comment'],
                'coordinated': ['все вместе', 'одновременно', 'координация', 'together', 'simultaneously']
            }
        }
    
    async def get_account_posts(self, account_id: int, limit: int = 100) -> List[Dict]:
        """Получение всех постов аккаунта из БД"""
        print(f"  📊 get_account_posts(account_id={account_id}, limit={limit})")
        
        if not self.db:
            print("  ⚠️ Нет подключения к БД")
            return []
        
        try:
            query = """
                SELECT id, content, published_at, likes_count, reposts_count, replies_count
                FROM posts
                WHERE account_id = %s
                ORDER BY published_at DESC
                LIMIT %s
            """
            
            cursor = self.db.cursor()
            cursor.execute(query, (account_id, limit))
            posts = cursor.fetchall()
            cursor.close()
            
            print(f"  ✅ Найдено {len(posts)} постов")
            
            return [{
                'id': p[0],
                'content': p[1],
                'published_at': p[2],
                'likes': p[3],
                'reposts': p[4],
                'replies': p[5]
            } for p in posts]
        except Exception as e:
            print(f"  ❌ Ошибка в get_account_posts: {e}")
            traceback.print_exc()
            return []
    
    async def get_post_comments(self, post_id: int) -> List[Dict]:
        """Получение комментариев к посту"""
        print(f"  💬 get_post_comments(post_id={post_id})")

        if not self.enable_live_comment_fetch:
            print("  ℹ️ Live comment fetch disabled")
            return []
        
        try:
            async with aiohttp.ClientSession() as session:
                url = f"https://mastodon.social/api/v1/statuses/{post_id}/context"
                async with session.get(url) as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        comments = data.get('descendants', [])
                        print(f"  ✅ Найдено {len(comments)} комментариев")
                        return comments
                    else:
                        print(f"  ⚠️ Mastodon API вернул {resp.status}")
                        return []
        except Exception as e:
            print(f"  ⚠️ Ошибка получения комментариев: {e}")
            return []
    
    async def find_similar_posts(self, content: str, threshold: float = 0.7) -> List[Tuple[int, float]]:
        """Поиск похожих постов в БД (координированные действия)"""
        print(f"  🔍 find_similar_posts(content_length={len(content)})")
        
        if not self.db:
            print("  ⚠️ Нет подключения к БД")
            return []
        
        try:
            query = """
                SELECT id, content FROM posts 
                WHERE content != ''
                ORDER BY published_at DESC 
                LIMIT 1000
            """
            cursor = self.db.cursor()
            cursor.execute(query)
            posts = cursor.fetchall()
            cursor.close()
            
            if not posts:
                print("  ⚠️ Нет постов для сравнения")
                return []
            
            print(f"  📊 Сравниваем с {len(posts)} постами")
            
            # Векторизация
            texts = [content] + [p[1] for p in posts]
            vectors = self.vectorizer.fit_transform(texts)
            
            # Поиск похожих
            similarities = cosine_similarity(vectors[0:1], vectors[1:])[0]
            
            similar = []
            for i, sim in enumerate(similarities):
                if sim > threshold:
                    similar.append((posts[i][0], float(sim)))
            
            result = sorted(similar, key=lambda x: x[1], reverse=True)[:10]
            print(f"  ✅ Найдено {len(result)} похожих постов")
            return result
            
        except Exception as e:
            print(f"  ❌ Ошибка в find_similar_posts: {e}")
            traceback.print_exc()
            return []
    
    async def analyze_account_behavior(self, account_id: int) -> Dict:
        """Анализ поведения аккаунта"""
        print(f"  📈 analyze_account_behavior(account_id={account_id})")
        
        try:
            posts = await self.get_account_posts(account_id, limit=50)
            
            if not posts:
                return {'suspicious': False, 'reason': 'Недостаточно данных'}
            
            # Анализ частоты постов
            post_times = [p['published_at'] for p in posts if p['published_at']]
            if len(post_times) > 1:
                time_diffs = [(post_times[i] - post_times[i+1]).total_seconds() / 3600 
                             for i in range(len(post_times)-1)]
                avg_gap = np.mean(time_diffs) if time_diffs else 0
                too_frequent = avg_gap < 0.5
            else:
                too_frequent = False
                avg_gap = 0
            
            # Анализ дублирования контента
            contents = [p['content'].lower() for p in posts]
            unique_hashes = set()
            duplicates = 0
            
            for cont in contents:
                content_hash = hashlib.md5(cont.encode()).hexdigest()
                if content_hash in unique_hashes:
                    duplicates += 1
                else:
                    unique_hashes.add(content_hash)
            
            duplicate_ratio = duplicates / len(contents) if contents else 0
            
            # Анализ времени публикации
            night_posts = 0
            for p in posts:
                if p['published_at']:
                    hour = p['published_at'].hour
                    if 0 <= hour <= 5:
                        night_posts += 1
            
            night_ratio = night_posts / len(posts) if posts else 0
            
            results = {
                'suspicious': too_frequent or duplicate_ratio > 0.3 or night_ratio > 0.5,
                'avg_time_gap_hours': round(avg_gap, 2),
                'duplicate_ratio': round(duplicate_ratio, 3),
                'night_posts_ratio': round(night_ratio, 3),
                'total_posts_analyzed': len(posts)
            }
            
            if too_frequent:
                results['reason'] = f'Слишком частая публикация (раз в {avg_gap:.1f} часов)'
            elif duplicate_ratio > 0.3:
                results['reason'] = f'Высокий уровень дублирования ({duplicate_ratio*100:.0f}%)'
            elif night_ratio > 0.5:
                results['reason'] = f'Подозрительная активность ночью ({night_ratio*100:.0f}% постов)'
            else:
                results['reason'] = 'Поведение аккаунта в пределах нормы'
            
            print(f"  ✅ suspicious={results['suspicious']}, reason={results['reason']}")
            return results
            
        except Exception as e:
            print(f"  ❌ Ошибка в analyze_account_behavior: {e}")
            traceback.print_exc()
            return {'suspicious': False, 'reason': f'Ошибка: {str(e)}'}
    
    async def analyze_coordination_network(self, post_id: int, similar_posts: List[Tuple[int, float]]) -> Dict:
        """Анализ координационной сети"""
        print(f"  🔗 analyze_coordination_network(similar_posts_count={len(similar_posts)})")
        
        if not similar_posts or not self.db:
            return {'coordinated_activity': False, 'unique_accounts': 0}
        
        try:
            post_ids = [p[0] for p in similar_posts[:10]]
            placeholders = ','.join(['%s'] * len(post_ids))
            
            query = f"""
                SELECT DISTINCT a.id, a.username, a.followers_count
                FROM posts p
                JOIN accounts a ON p.account_id = a.id
                WHERE p.id IN ({placeholders})
            """
            
            cursor = self.db.cursor()
            cursor.execute(query, post_ids)
            accounts = cursor.fetchall()
            cursor.close()
            
            unique_accounts = len(set([a[0] for a in accounts]))
            is_coordinated = unique_accounts >= 3 and len(similar_posts) >= 5
            
            result = {
                'coordinated_activity': is_coordinated,
                'unique_accounts': unique_accounts,
                'similar_posts_count': len(similar_posts),
                'accounts_involved': [{'id': a[0], 'username': a[1]} for a in accounts[:5]]
            }
            
            print(f"  ✅ coordinated={is_coordinated}, accounts={unique_accounts}")
            return result
            
        except Exception as e:
            print(f"  ❌ Ошибка в analyze_coordination_network: {e}")
            traceback.print_exc()
            return {'coordinated_activity': False, 'unique_accounts': 0}
    
    async def analyze_engagement_velocity(self, post_id: int, published_at: datetime) -> Dict:
        """Анализ скорости вовлечения"""
        print(f"  ⚡ analyze_engagement_velocity(post_id={post_id})")
        
        if not self.db:
            return {'suspicious_velocity': False}
        
        try:
            query = """
                SELECT likes_count, reposts_count, replies_count, published_at
                FROM posts
                WHERE id = %s
            """
            cursor = self.db.cursor()
            cursor.execute(query, (post_id,))
            post = cursor.fetchone()
            cursor.close()
            
            if not post or not published_at:
                return {'suspicious_velocity': False}
            
            likes, reposts, replies, pub_time = post
            
            age_hours = (datetime.now(timezone.utc) - self._normalize_datetime(published_at)).total_seconds() / 3600
            
            if age_hours < 1:
                return {'suspicious_velocity': False, 'age_hours': age_hours}
            
            engagement_rate = (likes + reposts * 2 + replies * 3) / max(1, age_hours)
            suspicious = engagement_rate > 100
            
            result = {
                'suspicious_velocity': suspicious,
                'engagement_rate_per_hour': round(engagement_rate, 2),
                'age_hours': round(age_hours, 1),
                'total_engagement': likes + reposts + replies
            }
            
            print(f"  ✅ suspicious={suspicious}, rate={engagement_rate:.1f}/hour")
            return result
            
        except Exception as e:
            print(f"  ❌ Ошибка в analyze_engagement_velocity: {e}")
            traceback.print_exc()
            return {'suspicious_velocity': False}
    
    async def analyze_comments_sentiment(self, post_id: int) -> Dict:
        """Анализ комментариев на предмет манипуляций"""
        print(f"  💬 analyze_comments_sentiment(post_id={post_id})")
        
        try:
            comments = await self.get_post_comments(post_id)
            
            if not comments:
                return {'has_comments': False, 'manipulation_in_comments': 0}
            
            manipulated_comments = 0
            comment_analysis = []
            
            for comment in comments[:50]:
                content = comment.get('content', '')
                if not content:
                    continue
                
                is_manipulative = False
                for category, patterns in self.manipulation_patterns.items():
                    for subcat, words in patterns.items():
                        for word in words:
                            if word.lower() in content.lower():
                                is_manipulative = True
                                break
                        if is_manipulative:
                            break
                    if is_manipulative:
                        break
                
                if is_manipulative:
                    manipulated_comments += 1
                    comment_analysis.append({
                        'content': content[:100],
                        'is_manipulative': True
                    })
            
            ratio = manipulated_comments / len(comments) if comments else 0
            
            result = {
                'has_comments': len(comments) > 0,
                'total_comments': len(comments),
                'manipulated_comments': manipulated_comments,
                'manipulation_ratio': round(ratio, 3),
                'comment_analysis': comment_analysis[:5]
            }
            
            print(f"  ✅ comments={len(comments)}, manipulated={manipulated_comments}")
            return result
            
        except Exception as e:
            print(f"  ❌ Ошибка в analyze_comments_sentiment: {e}")
            traceback.print_exc()
            return {'has_comments': False, 'manipulation_in_comments': 0}
    
    def _analyze_content_deep(self, text: str) -> Dict:
        print(f"  📝 _analyze_content_deep(text_length={len(text)})")
        
        try:
            text_lower = text.lower()
            
            # РАСШИРЕННЫЙ ПОДСЧЕТ МАРКЕРОВ
            emotional_words = ['ужас', 'шок', 'сенсация', 'срочно', 'важно', 
                            'внимание', 'скандал', 'разоблачение', 'кошмар']
            coordination_words = ['репост', 'ретвит', 'подпишись', 'расскажи', 
                                'поделись', 'лайкни', 'отправь']
            propaganda_words = ['скрывают правду', 'обманывают', 'заговор', 
                            'они хотят', 'правду']
            
            emotional_count = sum(1 for w in emotional_words if w in text_lower)
            coordination_count = sum(1 for w in coordination_words if w in text_lower)
            propaganda_count = sum(1 for w in propaganda_words if w in text_lower)
            
            # НОРМАЛИЗАЦИЯ С ВЕСАМИ (теперь каждый маркер дает больше)
            emotional_score = min(1.0, emotional_count / 3)  # 3 маркера = 100%
            coordination_score = min(1.0, coordination_count / 2)  # 2 маркера = 100%
            propaganda_score = min(1.0, propaganda_count / 1)  # 1 маркер = 100%
            
            # ИТОГОВАЯ ОЦЕНКА КОНТЕНТА (с бонусами)
            manipulation_score = (
                emotional_score * 0.4 +
                coordination_score * 0.35 +
                propaganda_score * 0.25
            )
            
            # БОНУС ЗА ВОСКЛИЦАНИЯ
            exclamation_count = text.count('!')
            if exclamation_count > 2:
                manipulation_score = min(1.0, manipulation_score + 0.15)
            
            # БОНУС ЗА CAPS
            caps_ratio = sum(1 for c in text if c.isupper()) / max(1, len(text))
            if caps_ratio > 0.2:
                manipulation_score = min(1.0, manipulation_score + 0.1)
            
            # БОНУС ЗА ДЛИНУ ТЕКСТА (короткие манипулятивные сообщения)
            if len(text) < 200 and manipulation_score > 0.3:
                manipulation_score = min(1.0, manipulation_score + 0.1)
            
            print(f"  ✅ emotional={emotional_count}, coordination={coordination_count}, propaganda={propaganda_count}")
            print(f"  ✅ manipulation_score={manipulation_score}")
            
            return {
                'manipulation_score': round(manipulation_score, 3),
                'emotional_score': round(emotional_score, 3),
                'fallacies_score': round(propaganda_score, 3),  # используем для пропаганды
                'coordination_score': round(coordination_score, 3),
                'exclamation_count': exclamation_count,
                'caps_ratio': round(caps_ratio, 3),
                'text_length': len(text)
            }
            
        except Exception as e:
            print(f"  ❌ Ошибка: {e}")
            return {
                'manipulation_score': 0.0,
                'emotional_score': 0,
                'fallacies_score': 0,
                'coordination_score': 0,
                'exclamation_count': 0,
                'caps_ratio': 0,
                'text_length': 0
            }
    
    def _normalize_datetime(self, value: datetime) -> datetime:
        if value is None:
            return datetime.now(timezone.utc)
        if value.tzinfo is None:
            return value.replace(tzinfo=timezone.utc)
        return value.astimezone(timezone.utc)

    def _analyze_account_factor(self, followers: int, created_at: datetime, behavior: Dict) -> Dict:
        try:
            if created_at:
                account_age_days = (datetime.now(timezone.utc) - self._normalize_datetime(created_at)).days
            else:
                account_age_days = 365
            
            is_new = account_age_days < 30
            has_few_followers = followers < 100
            
            if is_new and has_few_followers:
                score = 0.7
                reason = "Новый аккаунт с малым количеством подписчиков"
            elif is_new:
                score = 0.5
                reason = "Новый аккаунт"
            elif has_few_followers:
                score = 0.3
                reason = "Мало подписчиков"
            else:
                score = 0.1
                reason = "Аккаунт выглядит легитимным"
            
            return {
                'score': score,
                'reason': reason,
                'account_age_days': account_age_days,
                'followers': followers,
                'is_new': is_new
            }
        except Exception as e:
            print(f"  ❌ Ошибка: {e}")
            return {'score': 0.3, 'reason': 'Ошибка анализа', 'account_age_days': 0, 'followers': followers, 'is_new': False}
    
    def _generate_evidence_report(self, content: Dict, behavior: Dict, 
                                   coordination: Dict, comments: Dict,
                                   engagement: Dict) -> List[str]:
        """Генерация списка доказательств"""
        evidence = []
        
        try:
            if content.get('manipulation_score', 0) > 0.5:
                if content.get('emotional_score', 0) > 0.4:
                    evidence.append(f"🎭 Эмоциональная манипуляция (оценка: {content['emotional_score']:.0%})")
                if content.get('fallacies_score', 0) > 0.3:
                    evidence.append(f"🔍 Логические ошибки в аргументации ({content['fallacies_score']:.0%})")
                if content.get('exclamation_count', 0) > 3:
                    evidence.append(f"❗ Агрессивная пунктуация ({content['exclamation_count']} восклицаний)")
            
            if behavior.get('suspicious'):
                evidence.append(f"🤖 Подозрительное поведение аккаунта: {behavior.get('reason', '')}")
            
            if coordination.get('coordinated_activity'):
                evidence.append(f"🔄 Скоординированная кампания ({coordination['unique_accounts']} аккаунтов)")
            
            if comments.get('manipulation_ratio', 0) > 0.3:
                evidence.append(f"💬 Манипуляции в комментариях ({comments['manipulation_ratio']:.0%})")
            
            if engagement.get('suspicious_velocity'):
                evidence.append(f"⚡ Аномально высокая скорость вовлечения ({engagement['engagement_rate_per_hour']}/час)")
            
            if not evidence:
                evidence.append("✅ Явных признаков манипуляции не обнаружено")
        except Exception as e:
            print(f"  ❌ Ошибка в _generate_evidence_report: {e}")
            evidence = ["⚠️ Ошибка при генерации доказательств"]
        
        return evidence[:5]
    
    def _identify_advanced_tactics(self, content: Dict, coordination: Dict, behavior: Dict) -> List[str]:
        """Идентификация конкретных тактик манипуляции"""
        tactics = []
        
        try:
            if content.get('emotional_score', 0) > 0.5:
                tactics.append("🎯 Апелляция к эмоциям")
            if content.get('fallacies_score', 0) > 0.4:
                tactics.append("🎯 Логическая уловка")
            if coordination.get('coordinated_activity'):
                tactics.append("🎯 Скоординированная атака")
            if behavior.get('suspicious'):
                tactics.append("🎯 Автоматизированная активность")
            if content.get('caps_ratio', 0) > 0.3:
                tactics.append("🎯 Кричащий заголовок (CAPS LOCK)")
            if content.get('exclamation_count', 0) > 5:
                tactics.append("🎯 Эмоциональное давление")
        except Exception as e:
            print(f"  ❌ Ошибка в _identify_advanced_tactics: {e}")
        
        return tactics if tactics else ["🎯 Слабовыраженные признаки"]
    
    def _generate_recommendation(self, score: float, coordination: Dict) -> str:
        """Генерация рекомендации"""
        if score > 0.8:
            return "🔴 НЕМЕДЛЕННАЯ МОДЕРАЦИЯ: Пост содержит явные признаки скоординированной манипуляции"
        elif score > 0.6:
            return "🟠 ТРЕБУЕТ ПРОВЕРКИ: Рекомендуется ручная модерация"
        elif score > 0.4 or coordination.get('coordinated_activity'):
            return "🟡 ВНИМАНИЕ: Возможна манипуляция, рекомендуем наблюдение"
        else:
            return "🟢 БЕЗОПАСНО: Значимых признаков манипуляции не обнаружено"
    
    async def comprehensive_analysis(self, post_id: int, content: str, 
                                      account_id: int, username: str,
                                      published_at: datetime,
                                      followers_count: int,
                                      account_created_at: datetime) -> Dict:
        """ПОЛНЫЙ КОМПЛЕКСНЫЙ АНАЛИЗ поста"""
        
        print(f"\n{'='*60}")
        print(f"🔬 НАЧАЛО АНАЛИЗА поста #{post_id}")
        print(f"   Аккаунт: {username} (id={account_id})")
        print(f"   Подписчиков: {followers_count}")
        print(f"   Текст: {content[:100]}...")
        print(f"{'='*60}")
        
        results = {
            'post_id': post_id,
            'manipulation_score': 0.0,
            'confidence_score': 0.0,
            'components': {}
        }
        
        try:
            # 1. Анализ контента (текст)
            print("\n📝 1. Анализ контента...")
            content_analysis = self._analyze_content_deep(content)
            results['components']['content'] = content_analysis
            
            # 2. Анализ поведения аккаунта
            print("\n👤 2. Анализ поведения аккаунта...")
            account_behavior = await self.analyze_account_behavior(account_id)
            results['components']['account_behavior'] = account_behavior
            
            # 3. Поиск похожих постов (координация)
            print("\n🔄 3. Поиск похожих постов...")
            similar_posts = await self.find_similar_posts(content)
            coordination_network = await self.analyze_coordination_network(post_id, similar_posts)
            results['components']['coordination'] = coordination_network
            results['similar_posts'] = similar_posts[:5]
            
            # 4. Анализ комментариев
            print("\n💬 4. Анализ комментариев...")
            comments_analysis = await self.analyze_comments_sentiment(post_id)
            results['components']['comments'] = comments_analysis
            
            # 5. Анализ скорости вовлечения
            print("\n⚡ 5. Анализ скорости вовлечения...")
            engagement_velocity = await self.analyze_engagement_velocity(post_id, published_at)
            results['components']['engagement'] = engagement_velocity
            
            # 6. Фактор аккаунта
            print("\n📊 6. Фактор аккаунта...")
            account_factor = self._analyze_account_factor(
                followers_count, account_created_at, account_behavior
            )
            results['components']['account_factor'] = account_factor
            
            # ВЫЧИСЛЕНИЕ ИТОГОВОЙ ОЦЕНКИ
            print("\n🎯 7. Вычисление итоговой оценки...")
            
            weights = {
                'content': 0.35,
                'account_behavior': 0.20,
                'coordination': 0.25,
                'comments': 0.10,
                'engagement': 0.05,
                'account_factor': 0.05
            }
            
            scores = {}
            scores['content'] = content_analysis['manipulation_score']
            scores['account_behavior'] = 0.7 if account_behavior['suspicious'] else 0.2
            scores['coordination'] = 0.9 if coordination_network['coordinated_activity'] else (
                0.5 if coordination_network['unique_accounts'] > 1 else 0.1
            )
            scores['comments'] = comments_analysis.get('manipulation_ratio', 0)
            scores['engagement'] = 0.8 if engagement_velocity.get('suspicious_velocity') else 0.2
            scores['account_factor'] = account_factor['score']
            
            manipulation_score = sum(scores[comp] * weights[comp] for comp in weights)
            
            confidence_factors = []
            if similar_posts:
                confidence_factors.append(0.3)
            if comments_analysis['has_comments']:
                confidence_factors.append(0.2)
            
            account_posts = await self.get_account_posts(account_id, 10)
            if len(account_posts) > 5:
                confidence_factors.append(0.2)
            
            confidence_score = 0.5 + min(0.4, sum(confidence_factors))
            confidence_score = min(0.95, confidence_score)
            
            if manipulation_score > 0.7 and confidence_score > 0.7:
                escalation_priority = 1
                priority_text = "КРИТИЧЕСКИЙ"
            elif manipulation_score > 0.5 or coordination_network['coordinated_activity']:
                escalation_priority = 2
                priority_text = "ВЫСОКИЙ"
            else:
                escalation_priority = 3
                priority_text = "СРЕДНИЙ"
            
            evidence = self._generate_evidence_report(
                content_analysis, account_behavior, coordination_network, 
                comments_analysis, engagement_velocity
            )
            
            tactics = self._identify_advanced_tactics(
                content_analysis, coordination_network, account_behavior
            )
            
            results.update({
                'manipulation_score': round(manipulation_score, 3),
                'confidence_score': round(confidence_score, 3),
                'escalation_priority': escalation_priority,
                'priority_text': priority_text,
                'scores': scores,
                'key_evidence': evidence,
                'tactics': tactics,
                'recommendation': self._generate_recommendation(manipulation_score, coordination_network)
            })
            
            print(f"\n{'='*60}")
            print(f"📊 РЕЗУЛЬТАТЫ АНАЛИЗА поста #{post_id}")
            print(f"   Манипуляция: {manipulation_score*100:.1f}%")
            print(f"   Уверенность: {confidence_score*100:.1f}%")
            print(f"   Приоритет: {priority_text}")
            print(f"{'='*60}\n")
            
            return results
            
        except Exception as e:
            print(f"\n❌ КРИТИЧЕСКАЯ ОШИБКА в comprehensive_analysis: {e}")
            traceback.print_exc()
            
            results['error'] = str(e)
            results['manipulation_score'] = 0.5
            results['confidence_score'] = 0.3
            results['escalation_priority'] = 3
            results['key_evidence'] = [f"Ошибка анализа: {str(e)[:100]}"]
            results['tactics'] = ["Ошибка в анализе"]
            
            return results
