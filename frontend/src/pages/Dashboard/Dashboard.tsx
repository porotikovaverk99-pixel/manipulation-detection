import React, { useEffect, useState } from 'react';
import styles from './Dashboard.module.css';

interface AnalysisResult {
  id: number;
  postId: number;
  manipulationScore: number;
  confidenceScore: number;
  coordinationContribution: number;
  temporalContribution: number;
  narrativeContribution: number;
  escalationPriority: number;
  confidenceNote: string;
}

interface TrendingTag {
  id: number;
  tagName: string;
  todayUses: number;
}

const Dashboard: React.FC = () => {
  const [results, setResults] = useState<AnalysisResult[]>([]);
  const [trends, setTrends] = useState<TrendingTag[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    // Тестовые данные
    const mockResults: AnalysisResult[] = [
      {
        id: 1,
        postId: 1001,
        manipulationScore: 0.85,
        confidenceScore: 0.92,
        coordinationContribution: 0.78,
        temporalContribution: 0.65,
        narrativeContribution: 0.88,
        escalationPriority: 1,
        confidenceNote: 'Высокая уверенность, все три ветки подтверждают',
      },
      {
        id: 2,
        postId: 1002,
        manipulationScore: 0.45,
        confidenceScore: 0.60,
        coordinationContribution: 0.30,
        temporalContribution: 0.50,
        narrativeContribution: 0.55,
        escalationPriority: 2,
        confidenceNote: 'Средняя уверенность, конфликт между ветками',
      },
      {
        id: 3,
        postId: 1003,
        manipulationScore: 0.92,
        confidenceScore: 0.95,
        coordinationContribution: 0.90,
        temporalContribution: 0.85,
        narrativeContribution: 0.95,
        escalationPriority: 1,
        confidenceNote: 'Очень высокая уверенность',
      },
      {
        id: 4,
        postId: 1004,
        manipulationScore: 0.20,
        confidenceScore: 0.75,
        coordinationContribution: 0.15,
        temporalContribution: 0.25,
        narrativeContribution: 0.20,
        escalationPriority: 3,
        confidenceNote: 'Низкий риск',
      },
    ];

    const mockTrends: TrendingTag[] = [
      { id: 1, tagName: 'политика', todayUses: 156 },
      { id: 2, tagName: 'выборы2026', todayUses: 89 },
      { id: 3, tagName: 'россия', todayUses: 67 },
      { id: 4, tagName: 'пропаганда', todayUses: 45 },
      { id: 5, tagName: 'новости', todayUses: 234 },
    ];

    setResults(mockResults);
    setTrends(mockTrends);
    setLoading(false);
  }, []);

  if (loading) {
    return <div className={styles.loader}>Загрузка данных...</div>;
  }

  const highPriorityCount = results.filter(r => r.escalationPriority === 1).length;
  const avgConfidence = Math.round(
    results.reduce((acc, r) => acc + r.confidenceScore, 0) / results.length * 100
  );

  const getCardPriorityClass = (priority: number) => {
    switch (priority) {
      case 1: return styles.resultCardHigh;
      case 2: return styles.resultCardMedium;
      default: return styles.resultCardLow;
    }
  };

  return (
    <div className={styles.container}>
      <h1 className={styles.title}>
        🎯 Детекция манипулятивных тактик в соцсетях
      </h1>

      {/* Статистика */}
      <div className={styles.statsGrid}>
        <div className={`${styles.statCard} ${styles.statCardBlue}`}>
          <div className={styles.statValue}>{results.length}</div>
          <div className={styles.statLabel}>Проанализировано постов</div>
        </div>
        <div className={`${styles.statCard} ${styles.statCardRed}`}>
          <div className={styles.statValue}>{highPriorityCount}</div>
          <div className={styles.statLabel}>Высокий приоритет</div>
        </div>
        <div className={`${styles.statCard} ${styles.statCardGreen}`}>
          <div className={styles.statValue}>{avgConfidence}%</div>
          <div className={styles.statLabel}>Средняя уверенность</div>
        </div>
        <div className={`${styles.statCard} ${styles.statCardPurple}`}>
          <div className={styles.statValue}>3</div>
          <div className={styles.statLabel}>Ветки анализа</div>
        </div>
      </div>

      {/* Трендовые хэштеги */}
      <div className={styles.trendsSection}>
        <h2 className={styles.sectionTitle}>
          📈 Трендовые хэштеги
        </h2>
        <div className={styles.trendsList}>
          {trends.map((tag) => (
            <span key={tag.id} className={styles.trendTag}>
              #{tag.tagName}
              <span className={styles.trendCount}>{tag.todayUses}</span>
            </span>
          ))}
        </div>
      </div>

      {/* Результаты анализа */}
      <h2 className={styles.sectionTitle}>
        📊 Результаты анализа
      </h2>
      <div className={styles.resultsList}>
        {results.map((result) => (
          <div 
            key={result.id} 
            className={`${styles.resultCard} ${getCardPriorityClass(result.escalationPriority)}`}
          >
            <div className={styles.resultContent}>
              <div className={styles.resultLeft}>
                <div className={styles.postId}>Пост #{result.postId}</div>

                <div className={styles.branchesGrid}>
                  <div className={styles.branchItem}>
                    <div className={styles.branchLabel}>
                      <span className={styles.branchName}>🤝 Координация</span>
                      <span className={styles.branchValue}>
                        {Math.round(result.coordinationContribution * 100)}%
                      </span>
                    </div>
                    <div className={styles.branchBarBg}>
                      <div
                        className={`${styles.branchBarFill} ${styles.coordination}`}
                        style={{ width: `${result.coordinationContribution * 100}%` }}
                      />
                    </div>
                  </div>

                  <div className={styles.branchItem}>
                    <div className={styles.branchLabel}>
                      <span className={styles.branchName}>⏱️ Временные</span>
                      <span className={styles.branchValue}>
                        {Math.round(result.temporalContribution * 100)}%
                      </span>
                    </div>
                    <div className={styles.branchBarBg}>
                      <div
                        className={`${styles.branchBarFill} ${styles.temporal}`}
                        style={{ width: `${result.temporalContribution * 100}%` }}
                      />
                    </div>
                  </div>

                  <div className={styles.branchItem}>
                    <div className={styles.branchLabel}>
                      <span className={styles.branchName}>📖 Нарратив</span>
                      <span className={styles.branchValue}>
                        {Math.round(result.narrativeContribution * 100)}%
                      </span>
                    </div>
                    <div className={styles.branchBarBg}>
                      <div
                        className={`${styles.branchBarFill} ${styles.narrative}`}
                        style={{ width: `${result.narrativeContribution * 100}%` }}
                      />
                    </div>
                  </div>
                </div>

                {result.confidenceNote && (
                  <div className={styles.confidenceNote}>
                    💡 {result.confidenceNote}
                  </div>
                )}
              </div>

              <div className={styles.resultRight}>
                <div className={styles.manipulationScore}>
                  {Math.round(result.manipulationScore * 100)}%
                </div>
                <div className={styles.manipulationLabel}>манипуляция</div>
                <div className={`${styles.priorityBadge} ${
                  result.escalationPriority === 1 ? styles.priorityHigh :
                  result.escalationPriority === 2 ? styles.priorityMedium : styles.priorityLow
                }`}>
                  {result.escalationPriority === 1 ? '🔴 Высокий приоритет' :
                   result.escalationPriority === 2 ? '🟡 Средний приоритет' : '🟢 Низкий приоритет'}
                </div>
              </div>
            </div>
          </div>
        ))}
      </div>

      {/* Информационная панель */}
      <div className={styles.infoPanel}>
        <div className={styles.infoTitle}>
          📋 О методе анализа
        </div>
        <div className={styles.infoText}>
          Анализ выполняется по трём независимым веткам:{' '}
          <strong>координация</strong> (совместная активность, повторяющиеся ссылки),{' '}
          <strong>временные паттерны</strong> (всплески активности, задержки между постами) и{' '}
          <strong>нарратив</strong> (повторяющиеся фреймы, сдвиги формулировок).<br />
          Итоговая оценка манипуляции и приоритет эскалации формируются на основе вклада всех трёх веток.
        </div>
      </div>
    </div>
  );
};

export default Dashboard;