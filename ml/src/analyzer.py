import re
import math
from typing import List, Dict, Tuple
from collections import Counter

class ManipulationAnalyzer:
    def __init__(self):
        # Манипулятивные маркеры (русский язык)
        self.emotional_markers = [
            'ужас', 'шок', 'сенсация', 'невероятно', 'срочно',
            'важно', 'внимание', 'скандал', 'разоблачение'
        ]
        
        self.logical_fallacies = [
            'все знают', 'никто не', 'очевидно', 'бесспорно',
            'ясно как день', 'факт', 'правда'
        ]
        
        self.coordination_markers = [
            'подписывайтесь', 'репост', 'распространите',
            'расскажите всем', 'поделитесь'
        ]
        
        self.propaganda_phrases = [
            'они хотят', 'нас обманывают', 'скрывают правду',
            'запад', 'власть', 'режим', 'оппозиция'
        ]
        
        self.fake_news_markers = [
            'инсайд', 'источник', 'слив', 'как стало известно',
            'по данным', 'якобы', 'говорят'
        ]
    
    def analyze_text(self, text: str) -> Dict:
        text_lower = text.lower()
        
        # 1. Нарративный анализ (контент)
        emotional_score = self._calculate_emotional_score(text_lower)
        logical_score = self._calculate_logical_score(text_lower)
        propaganda_score = self._calculate_propaganda_score(text_lower)
        fake_score = self._calculate_fake_score(text_lower)
        
        narrative_score = (emotional_score + logical_score + propaganda_score + fake_score) / 4
        
        # 2. Координационный анализ (призывы к действию)
        coordination_score = self._calculate_coordination_score(text_lower)
        
        # 3. Темпоральный анализ (срочность)
        temporal_score = self._calculate_temporal_score(text_lower)
        
        # Итоговая оценка
        manipulation_score = (narrative_score * 0.5 + 
                              coordination_score * 0.25 + 
                              temporal_score * 0.25)
        
        # Уверенность
        confidence_score = min(0.95, 0.5 + manipulation_score * 0.5)
        
        # Определение тактик
        tactics = self._identify_tactics(emotional_score, logical_score, 
                                          propaganda_score, fake_score)
        
        # Ключевые доказательства
        key_evidence = self._extract_evidence(text_lower)
        
        # Нота уверенности
        confidence_note = self._generate_confidence_note(
            narrative_score, coordination_score, temporal_score
        )
        
        return {
            'manipulation_score': round(manipulation_score, 3),
            'confidence_score': round(confidence_score, 3),
            'coordination_contribution': round(coordination_score, 3),
            'temporal_contribution': round(temporal_score, 3),
            'narrative_contribution': round(narrative_score, 3),
            'confidence_note': confidence_note,
            'key_evidence': key_evidence,
            'tactics': tactics
        }
    
    def _calculate_emotional_score(self, text: str) -> float:
        """Эмоциональное давление"""
        count = sum(1 for marker in self.emotional_markers if marker in text)
        return min(1.0, count / 5)
    
    def _calculate_logical_score(self, text: str) -> float:
        """Логические ошибки"""
        count = sum(1 for marker in self.logical_fallacies if marker in text)
        return min(1.0, count / 4)
    
    def _calculate_propaganda_score(self, text: str) -> float:
        """Пропагандистские приёмы"""
        count = sum(1 for phrase in self.propaganda_phrases if phrase in text)
        return min(1.0, count / 3)
    
    def _calculate_fake_score(self, text: str) -> float:
        """Признаки фейка"""
        count = sum(1 for marker in self.fake_news_markers if marker in text)
        return min(1.0, count / 3)
    
    def _calculate_coordination_score(self, text: str) -> float:
        """Призывы к координации"""
        count = sum(1 for marker in self.coordination_markers if marker in text)
        return min(1.0, count / 3)
    
    def _calculate_temporal_score(self, text: str) -> float:
        """Срочность, временное давление"""
        urgency_markers = ['срочно', 'немедленно', 'прямо сейчас', 'сегодня же']
        count = sum(1 for marker in urgency_markers if marker in text)
        return min(1.0, count / 2)
    
    def _identify_tactics(self, emotional: float, logical: float, 
                          propaganda: float, fake: float) -> List[str]:
        tactics = []
        if emotional > 0.5:
            tactics.append('эмоциональное давление')
        if logical > 0.4:
            tactics.append('логическая ошибка')
        if propaganda > 0.4:
            tactics.append('пропагандистский приём')
        if fake > 0.3:
            tactics.append('манипуляция фактами')
        if not tactics:
            tactics.append('слабо выраженные признаки')
        return tactics
    
    def _extract_evidence(self, text: str) -> List[str]:
        evidence = []
        for marker in self.emotional_markers[:3]:
            if marker in text:
                evidence.append(f'Эмоциональный маркер: "{marker}"')
                break
        
        for marker in self.coordination_markers[:2]:
            if marker in text:
                evidence.append(f'Призыв к действию: "{marker}"')
                break
        
        if any(phrase in text for phrase in self.propaganda_phrases):
            evidence.append('Обнаружены пропагандистские конструкции')
        
        if not evidence:
            evidence.append('Выраженных манипулятивных маркеров не обнаружено')
        
        return evidence[:3]
    
    def _generate_confidence_note(self, narrative: float, 
                                   coordination: float, 
                                   temporal: float) -> str:
        if narrative > 0.7 and coordination > 0.5:
            return "Высокая уверенность: нарративные и координационные сигналы совпадают"
        elif narrative > 0.7:
            return "Средняя уверенность: сильные нарративные сигналы, слабая координация"
        elif coordination > 0.5:
            return "Средняя уверенность: координационные сигналы без нарративного подтверждения"
        elif abs(narrative - coordination) > 0.4:
            return "Низкая уверенность: конфликт между ветками анализа"
        else:
            return "Требуется дополнительный анализ: недостаточно сигналов"

# Глобальный экземпляр
analyzer = ManipulationAnalyzer()