import styles from './Breakdown.module.css';
import { formatScore } from '../../../utils/format';

export function Breakdown({ label, value }) {
  return (
    <div className={`${styles.root} breakdown-row`}>
      <span>{label}</span>
      <div className="breakdown-track">{value !== null && <span style={{ width: `${value * 100}%` }} />}</div>
      <strong className="mono">{formatScore(value)}</strong>
    </div>
  );
}
