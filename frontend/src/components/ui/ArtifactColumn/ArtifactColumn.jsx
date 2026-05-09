import styles from './ArtifactColumn.module.css';

export function ArtifactColumn({ title, items }) {
  return (
    <div className={styles.root}>
      <h3>{title}</h3>
      {items.length === 0 && <p className="empty">No values.</p>}
      {items.map(([value, count]) => (
        <div className="artifact-row" key={value}>
          <span>{value}</span>
          <strong>x{count}</strong>
        </div>
      ))}
    </div>
  );
}
