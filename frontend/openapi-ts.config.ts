import {defineConfig} from '@hey-api/openapi-ts'

const input = process.env.UNIVERSALTASKS_OPENAPI_INPUT
if (!input) {
	throw new Error('UNIVERSALTASKS_OPENAPI_INPUT must point to the generated temporary spec')
}

export default defineConfig({
	input,
	output: 'src/client/generated',
	plugins: [
		'@hey-api/typescript',
		'@hey-api/sdk',
		{
			name: '@hey-api/client-fetch',
			throwOnError: true,
		},
	],
})
