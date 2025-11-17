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
});

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
