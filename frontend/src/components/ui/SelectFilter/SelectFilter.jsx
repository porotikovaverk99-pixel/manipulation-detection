import styles from './SelectFilter.module.css';

export function SelectFilter({ label, value, options, onChange }) {
  return (
    <label className={`${styles.root} select-chip`}>
      <span>{label}</span>
      <select value={value} onChange={(event) => onChange(event.target.value)}>
        {options.map((option) => (
          <option key={option} value={option}>
            {option}
          </option>
        ))}
      </select>
    </label>
  );
}
