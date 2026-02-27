import axios from 'axios'
import { login } from '../auth'

const client = axios.create({
  baseURL: '/api/v1',
  timeout: 15000,
})

// Attach JWT from localStorage
client.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// On 401: initiate OIDC PKCE login flow
client.interceptors.response.use(
  (res) => res,
  (err) => {
    if (err.response?.status === 401) {
      // Don't redirect if already on /callback (avoids loop)
      if (!window.location.pathname.startsWith('/callback')) {
        login()
      }
    }
    return Promise.reject(err)
  }
)

export default client
