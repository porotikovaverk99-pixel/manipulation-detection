import styles from './SummaryTile.module.css';

export function SummaryTile({ label, value, note, tone }) {
  return (
    <div className={`${styles.root} tile ${tone || ''}`}>
      <span>{label}</span>
      <strong>{value}</strong>
      <small>{note}</small>
    </div>
  );
}
