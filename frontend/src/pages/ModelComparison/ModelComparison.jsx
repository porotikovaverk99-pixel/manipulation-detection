import { useEffect, useState } from 'react';
import { getModelComparison } from '../../services/api';
import { MetricRow, PanelTitle, StateMessage, SummaryTile, Weight } from '../../components/ui';
import { bestBy, formatScore } from '../../utils/format';
import styles from './ModelComparison.module.css';

export function ModelComparison() {
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    getModelComparison()
      .then((response) => {
        if (!cancelled) setData(response);
      })
      .catch((err) => {
        if (!cancelled) setError(err.message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (loading) return <StateMessage title="Loading metrics" text="Reading model comparison from backend." />;
  if (error || !data) return <StateMessage title="Evaluation unavailable" text={error || 'No evaluation data.'} />;

  const bestPr = bestBy(data.rows, 'pr_auc');
  const bestRoc = bestBy(data.rows, 'roc_auc');

  return (
    <section className={`${styles.root} content`}>
      <div className="page-head">
        <h1>Model comparison</h1>
        <p>
          Runtime comparison from <span className="mono">case_model_scores</span> for{' '}
          <span className="mono">{data.dataset_name}/{data.dataset_split}</span>.
        </p>
      </div>

      <div className="tiles eval-tiles">
        <SummaryTile label="Eval cases" value={data.case_count} note={`${data.positive_cases} positive`} />
        <SummaryTile label="Best ranking model" value={bestPr?.label || 'none'} note="by PR-AUC" />
        <SummaryTile label="Best ROC-AUC" value={formatScore(bestRoc?.metrics.roc_auc)} note={bestRoc?.label || ''} />
        <SummaryTile label="Best PR-AUC" value={formatScore(bestPr?.metrics.pr_auc)} note={bestPr?.label || ''} />
      </div>

      <section className="panel">
        <PanelTitle title="Metrics table" />
        <div className="metrics-table">
          <div className="metrics-row head">
            <div>Model</div>
            <div>P@10</div>
            <div>P@20</div>
            <div>Recall</div>
            <div>F1</div>
            <div>ROC-AUC</div>
            <div>PR-AUC</div>
          </div>
          {data.rows.map((row) => <MetricRow key={row.scorer_key} row={row} rows={data.rows} />)}
        </div>
      </section>

      <section className="panel">
        <PanelTitle title="Ensemble weights" subtitle="case_ensemble_v1" />
        {data.rows
          .filter((row) => row.ensemble_weights)
          .map((row) => (
            <div key={row.scorer_key} className="weights">
              <Weight label="Transformer" value={row.ensemble_weights?.transformer || 0} />
              <Weight label="LightGBM" value={row.ensemble_weights?.lightgbm || 0} />
              <Weight label="Logistic baseline" value={row.ensemble_weights?.baseline || 0} />
            </div>
          ))}
      </section>
    </section>
  );
}
