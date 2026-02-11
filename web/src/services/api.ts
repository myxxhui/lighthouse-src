import axios, { AxiosInstance, AxiosRequestConfig, AxiosResponse } from 'axios';
import { ApiError } from '@/types';

const API_BASE_URL = process.env.API_BASE_URL || '/api';

class ApiClient {
  private client: AxiosInstance;

  constructor() {
    this.client = axios.create({
      baseURL: API_BASE_URL,
      timeout: 10000,
      headers: {
        'Content-Type': 'application/json',
      },
    });

    // 请求拦截器
    this.client.interceptors.request.use(
      config => {
        // 可以在这里添加认证token等
        return config;
      },
      error => {
        return Promise.reject(error);
      },
    );

    // 响应拦截器
    this.client.interceptors.response.use(
      response => {
        return response;
      },
      error => {
        const apiError: ApiError = {
          message: error.response?.data?.message || error.message || '未知错误',
          code: error.response?.data?.code || 'UNKNOWN_ERROR',
          timestamp: new Date().toISOString(),
        };
        return Promise.reject(apiError);
      },
    );
  }

  public get<T>(url: string, config?: AxiosRequestConfig): Promise<AxiosResponse<T>> {
    return this.client.get<T>(url, config);
  }

  public post<T>(url: string, data?: any, config?: AxiosRequestConfig): Promise<AxiosResponse<T>> {
    return this.client.post<T>(url, data, config);
  }

  public put<T>(url: string, data?: any, config?: AxiosRequestConfig): Promise<AxiosResponse<T>> {
    return this.client.put<T>(url, data, config);
  }

  public delete<T>(url: string, config?: AxiosRequestConfig): Promise<AxiosResponse<T>> {
    return this.client.delete<T>(url, config);
  }
}

export const apiClient = new ApiClient();
