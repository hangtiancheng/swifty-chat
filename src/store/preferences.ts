import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

export interface PreferencesState {
  /** Kept in localStorage so it survives the sessionStorage-scoped auth state. */
  rememberedPhone: string;
  setRememberedPhone: (phone: string) => void;
}

const usePreferencesStore = create<PreferencesState>()(
  persist(
    (set) => ({
      rememberedPhone: "",
      setRememberedPhone: (rememberedPhone) => set({ rememberedPhone }),
    }),
    {
      name: "swifty-preferences",
      storage: createJSONStorage(() => localStorage),
    },
  ),
);

export default usePreferencesStore;
