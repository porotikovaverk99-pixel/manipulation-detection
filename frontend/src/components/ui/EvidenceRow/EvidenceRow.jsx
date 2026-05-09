import styles from './EvidenceRow.module.css';
import { formatScore } from '../../../utils/format';

export function EvidenceRow({ item }) {
  return (
    <div className={`${styles.root} evidence-row`}>
      <div>
        <strong>{item.label}</strong>
        <small>
          {item.key} · {item.group}
        </small>
      </div>
      <span className="mono">{item.value === null ? 'value' : formatScore(item.value)}</span>
    </div>
  );
}
