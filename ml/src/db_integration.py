import psycopg2
from psycopg2.extras import Json, RealDictCursor
from datetime import datetime
from typing import List, Dict, Optional

class DatabaseConnector:
    """Интеграция с PostgreSQL для получения данных аккаунтов и постов"""
    
    def __init__(self, conn_string: str):
        self.conn_string = conn_string
        self.connection = None
    
    def connect(self):
        """Установка соединения с БД"""
        self.connection = psycopg2.connect(self.conn_string)
        return self.connection
    
    def get_account_posts(self, account_id: int, limit: int = 100) -> List[Dict]:
        try:
            with self.connection.cursor(cursor_factory=RealDictCursor) as cur:
                cur.execute("""
                    SELECT id, content, published_at, likes_count, reposts_count, replies_count
                    FROM posts
                    WHERE account_id = %s
                    ORDER BY published_at DESC
                    LIMIT %s
                """, (account_id, limit))
                return cur.fetchall()
        except Exception as e:
            print(f"❌ Ошибка в get_account_posts: {e}")
            return []

    def get_account_info(self, account_id: int) -> Optional[Dict]:
        try:
            with self.connection.cursor(cursor_factory=RealDictCursor) as cur:
                cur.execute("""
                    SELECT id, username, display_name, followers_count, following_count,
                        posts_count, created_at, is_bot
                    FROM accounts
                    WHERE id = %s
                """, (account_id,))
                return cur.fetchone()
        except Exception as e:
            print(f"❌ Ошибка в get_account_info: {e}")
            return None
    
    def get_similar_posts_by_content(self, content: str, limit: int = 100) -> List[Dict]:
        """Поиск похожих постов по содержанию"""
        with self.connection.cursor(cursor_factory=RealDictCursor) as cur:
            # Используем триграммы PostgreSQL для поиска похожих текстов
            cur.execute("""
                SELECT id, content, account_id, published_at,
                       similarity(content, %s) as similarity
                FROM posts
                WHERE content % %s
                ORDER BY similarity DESC
                LIMIT %s
            """, (content, content, limit))
            return cur.fetchall()
    
    def get_posts_by_time_range(self, start_time: datetime, end_time: datetime) -> List[Dict]:
        """Получение постов за временной промежуток (для анализа всплесков)"""
        with self.connection.cursor(cursor_factory=RealDictCursor) as cur:
            cur.execute("""
                SELECT p.id, p.content, p.account_id, p.published_at,
                       a.username, a.followers_count
                FROM posts p
                JOIN accounts a ON p.account_id = a.id
                WHERE p.published_at BETWEEN %s AND %s
                ORDER BY p.published_at
            """, (start_time, end_time))
            return cur.fetchall()
    
    def get_post_comments_from_db(self, post_id: int) -> List[Dict]:
        """Получение комментариев (если сохраняются в БД)"""
        # Если комментарии сохраняются в отдельной таблице
        with self.connection.cursor(cursor_factory=RealDictCursor) as cur:
            cur.execute("""
                SELECT id, content, account_id, created_at
                FROM comments
                WHERE post_id = %s
                ORDER BY created_at
                LIMIT 100
            """, (post_id,))
            return cur.fetchall()
    
    def save_analysis_result(self, post_id: int, analysis_result: Dict):
        """Сохранение результатов анализа в БД"""
        try:
            with self.connection.cursor() as cur:
                # ПРОВЕРЯЕМ: существует ли уже запись
                cur.execute("SELECT id FROM analysis_results WHERE post_id = %s", (post_id,))
                existing = cur.fetchone()
                
                if existing:
                    # ОБНОВЛЯЕМ существующую запись
                    cur.execute("""
                        UPDATE analysis_results 
                        SET manipulation_score = %s,
                            confidence_score = %s,
                            coordination_contribution = %s,
                            temporal_contribution = %s,
                            narrative_contribution = %s,
                            escalation_priority = %s,
                            confidence_note = %s,
                            updated_at = NOW()
                        WHERE post_id = %s
                        RETURNING id
                    """, (
                        analysis_result.get('manipulation_score', 0),
                        analysis_result.get('confidence_score', 0),
                        analysis_result.get('scores', {}).get('coordination', 0),
                        analysis_result.get('scores', {}).get('engagement', 0),
                        analysis_result.get('scores', {}).get('content', 0),
                        analysis_result.get('escalation_priority', 3),
                        analysis_result.get('recommendation', ''),
                        post_id
                    ))
                    result = cur.fetchone()
                    analysis_id = result[0] if result else existing[0]
                    print(f"🔄 Обновлен результат для поста {post_id}")
                    
                else:
                    # ВСТАВЛЯЕМ новую запись
                    cur.execute("""
                        INSERT INTO analysis_results (
                            post_id, manipulation_score, confidence_score,
                            coordination_contribution, temporal_contribution,
                            narrative_contribution, escalation_priority,
                            confidence_note, created_at
                        ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, NOW())
                        RETURNING id
                    """, (
                        post_id,
                        analysis_result.get('manipulation_score', 0),
                        analysis_result.get('confidence_score', 0),
                        analysis_result.get('scores', {}).get('coordination', 0),
                        analysis_result.get('scores', {}).get('engagement', 0),
                        analysis_result.get('scores', {}).get('content', 0),
                        analysis_result.get('escalation_priority', 3),
                        analysis_result.get('recommendation', '')
                    ))
                    result = cur.fetchone()
                    analysis_id = result[0] if result else None
                    print(f"✅ Создан новый результат для поста {post_id}")
                
                # Сохраняем evidence_cards в схему из backend/migrations.
                if analysis_id and analysis_result.get('key_evidence'):
                    cur.execute("""
                        INSERT INTO evidence_cards (analysis_result_id, key_evidence, summary, created_at)
                        VALUES (%s, %s, %s, NOW())
                        ON CONFLICT (analysis_result_id) DO UPDATE SET
                            key_evidence = EXCLUDED.key_evidence,
                            summary = EXCLUDED.summary
                    """, (
                        analysis_id,
                        Json(analysis_result['key_evidence']),
                        analysis_result.get('recommendation', '')
                    ))
                    print("  ✅ Карточка доказательств сохранена")
                
                # Фиксируем транзакцию
                self.connection.commit()
                print(f"✅ Все данные сохранены для поста {post_id}")
                
        except Exception as e:
            print(f"❌ Ошибка при сохранении результата: {e}")
            self.connection.rollback()
            raise
