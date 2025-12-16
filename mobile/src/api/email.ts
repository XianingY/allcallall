import axios, { AxiosError } from "axios";
import { API_BASE_URL, REQUEST_TIMEOUT } from "../config";

// API 响应类型定义
export interface ApiResponse<T> {
  data?: T;
  message?: string;
}

export interface SendVerificationCodeResponse {
  message: string;
}

export interface VerifyCodeResponse {
  message: string;
}

// 创建 API 实例
const apiClient = axios.create({
  baseURL: API_BASE_URL,
  timeout: REQUEST_TIMEOUT,
  headers: {
    'Content-Type': 'application/json',
    'Accept': 'application/json',
  },
});

// 添加请求拦截器
apiClient.interceptors.request.use(
  (config) => {
    console.log('[Email API] Request:', {
      url: config.url,
      method: config.method,
      baseURL: config.baseURL,
      data: config.data,
    });
    return config;
  },
  (error) => {
    console.error('[Email API] Request error:', error);
    return Promise.reject(error);
  }
);

// 添加响应拦截器
apiClient.interceptors.response.use(
  (response) => {
    console.log('[Email API] Response success:', {
      status: response.status,
      data: response.data,
    });
    return response;
  },
  (error) => {
    console.error('[Email API] Response error:', {
      message: error.message,
      response: error.response?.data,
      status: error.response?.status,
    });
    return Promise.reject(error);
  }
);

/**
 * 发送邮箱验证码
 * @param email 邮箱地址
 */
export const sendVerificationCode = async (email: string): Promise<void> => {
  try {
    const response = await apiClient.post<ApiResponse<SendVerificationCodeResponse>>(
      "/email/send-verification-code",
      { email }
    );
    console.log("[Email API] Send code response:", response.data);
  } catch (error) {
    const axiosError = error as AxiosError<{ message?: string }>;
    console.error("[Email API] Send code failed:", axiosError.response?.data);
    throw error;
  }
};

/**
 * 验证邮箱验证码
 * @param email 邮箱地址
 * @param code 6位验证码
 */
export const verifyCode = async (email: string, code: string): Promise<void> => {
  try {
    const response = await apiClient.post<ApiResponse<VerifyCodeResponse>>(
      "/email/verify-code",
      { email, code }
    );
    console.log("[Email API] Verify code response:", response.data);
  } catch (error) {
    const axiosError = error as AxiosError<{ message?: string }>;
    console.error("[Email API] Verify code failed:", axiosError.response?.data);
    throw error;
  }
};
