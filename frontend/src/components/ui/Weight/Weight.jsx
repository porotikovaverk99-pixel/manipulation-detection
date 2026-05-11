import styles from './Weight.module.css';
import { formatScore } from '../../../utils/format';

export function Weight({ label, value }) {
  return (
    <div className={`${styles.root} weight-row`}>
      <span>{label}</span>
      <div>
        <span style={{ width: `${value * 100}%` }} />
      </div>
      <strong className="mono">{formatScore(value)}</strong>
    </div>
  );
}
