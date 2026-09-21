import vuetify from 'eslint-config-vuetify'

export default vuetify(
  { ts: true },
  {
    ignores: [
      // 生成物不可手改（frontend/AGENTS.md），跳过 lint
      'src/api/generated/**',
      'src/api/generated-client/**',
      'src/typed-router.d.ts',
    ],
  },
)
