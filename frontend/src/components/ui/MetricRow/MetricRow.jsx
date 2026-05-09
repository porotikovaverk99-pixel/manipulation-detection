import styles from './MetricRow.module.css';
import { formatMetric, maxMetric } from '../../../utils/format';

export function MetricRow({ row, rows }) {
  const values = [
    ['precision_at_10', row.metrics.precision_at_10],
    ['precision_at_20', row.metrics.precision_at_20],
    ['recall', row.metrics.recall],
    ['f1', row.metrics.f1],
    ['roc_auc', row.metrics.roc_auc],
    ['pr_auc', row.metrics.pr_auc],
  ];

  return (
    <div className={`${styles.root} metrics-row`}>
      <div className="model-name">
        <strong>{row.label}</strong>
        <small>{row.scorer_key}</small>
      </div>
      {values.map(([key, value]) => (
        <div key={key} className={value === maxMetric(rows, key) ? 'best metric' : 'metric'}>
          {formatMetric(value)}
        </div>
      ))}
    </div>
  );
}
