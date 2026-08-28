/**
 * Copyright (c) 2026 hangtiancheng
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

import type { AuthResult, UserInfo } from "@/service/schemas";

export const emptyUser: UserInfo = {
  uuid: "",
  nickname: "",
  telephone: "",
  email: "",
  avatar: "",
  gender: 0,
  birthday: "",
  signature: "",
  status: 0,
  is_admin: 0,
  created_at: "",
};

export interface AuthState {
  token: string;
  userInfo: UserInfo;
  setAuth: (result: AuthResult) => void;
  setUserInfo: (info: UserInfo) => void;
  clearAuth: () => void;
}

const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: "",
      userInfo: emptyUser,
      setAuth: (result) =>
        set({ token: result.token, userInfo: result.user_info }),
      setUserInfo: (userInfo) => set({ userInfo }),
      clearAuth: () => set({ token: "", userInfo: emptyUser }),
    }),
    {
      name: "swifty-auth",
      storage: createJSONStorage(() => sessionStorage),
      partialize: (state) => ({ token: state.token, userInfo: state.userInfo }),
    },
  ),
);

export const selectIsLoggedIn = (state: AuthState) =>
  Boolean(state.token && state.userInfo.uuid);

export const isAuthenticated = () => selectIsLoggedIn(useAuthStore.getState());

export const currentUserId = () => useAuthStore.getState().userInfo.uuid;

export default useAuthStore;
