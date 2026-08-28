import { debounce } from "es-toolkit";
import { useEffect, useState } from "react";

/** Keeps server-side search from firing on every keystroke. */
export function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value);

  useEffect(() => {
    const update = debounce(setDebounced, delayMs);
    update(value);
    return () => update.cancel();
  }, [value, delayMs]);

  return debounced;
}
