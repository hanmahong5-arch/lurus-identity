import axios from 'axios'

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

// Redirect to Zitadel on 401
client.interceptors.response.use(
  (res) => res,
  (err) => {
    if (err.response?.status === 401) {
      window.location.href = 'https://auth.lurus.cn'
    }
    return Promise.reject(err)
  }
)

export default client
