import axios, { AxiosInstance } from "axios";

import { API_BASE_URL, REQUEST_TIMEOUT } from "../config";

export const createApiClient = (token?: string): AxiosInstance => {
  const instance = axios.create({
    baseURL: API_BASE_URL,
    timeout: REQUEST_TIMEOUT
  });

  instance.interceptors.request.use((config) => {
    if (token) {
      config.headers.Authorization = `Bearer ${token}`;
    }
    config.headers["Content-Type"] = "application/json";
    config.headers["Accept"] = "application/json";
    return config;
  });

  return instance;
};
