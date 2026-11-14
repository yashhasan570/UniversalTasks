import type {OAuthTokens} from '@/types/desktop'

export function isDesktopApp(): boolean {
	return !!window.universaltasksDesktop?.isDesktop
}

export function startDesktopOAuthLogin(apiUrl: string): Promise<void> {
	return window.universaltasksDesktop!.startOAuthLogin(apiUrl)
}

export function listenForDesktopOAuthTokens(callback: (tokens: OAuthTokens) => void): void {
	window.universaltasksDesktop!.onOAuthTokens(callback)
}

export function listenForDesktopOAuthError(callback: (error: string) => void): void {
	window.universaltasksDesktop!.onOAuthError(callback)
}

export function refreshDesktopToken(apiUrl: string, refreshToken: string): Promise<OAuthTokens> {
	return window.universaltasksDesktop!.refreshToken(apiUrl, refreshToken)
}
