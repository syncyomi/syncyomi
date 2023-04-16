const logLevel = ["DEBUG", "INFO", "WARN", "ERROR", "TRACE"] as const;

export const LogLevelOptions = logLevel.map(v => ({ value: v, title: v, key: v }));
