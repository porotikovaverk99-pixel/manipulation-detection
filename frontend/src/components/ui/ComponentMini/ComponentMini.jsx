import styles from './ComponentMini.module.css';
import { barHeight, formatScore } from '../../../utils/format';

export function ComponentMini({ scores }) {
  return (
    <div className={`${styles.root} component-mini`}>
      <span style={{ height: barHeight(scores.temporal_score) }} />
      <span style={{ height: barHeight(scores.coordination_score) }} />
      <span style={{ height: barHeight(scores.content_score) }} />
      <small>
        {formatScore(scores.temporal_score)} · {formatScore(scores.coordination_score)} · {formatScore(scores.content_score)}
      </small>
    </div>
  );
}
