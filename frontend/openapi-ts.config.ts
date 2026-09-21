import { defineConfig } from '@hey-api/openapi-ts'

export default defineConfig([
  {
    input: '.openapi/admin.json',
    output: {
      path: 'src/api/generated',
    },
    plugins: ['@hey-api/client-axios'],
  },
  {
    input: '.openapi/client.json',
    output: {
      path: 'src/api/generated-client',
    },
    plugins: ['@hey-api/client-axios'],
  },
])
