package io.github.chicohaager.moviescast.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

private val Accent = Color(0xFFFB923C)

// The colours of movies-cast's web pages (tv.css): a near-black blue and a warm orange.
private val Dark = darkColorScheme(
    primary = Accent,
    onPrimary = Color(0xFF0F121A),
    primaryContainer = Color(0xFF7C2D12),
    onPrimaryContainer = Color(0xFFFFEDD5),
    secondary = Color(0xFFFDBA74),
    background = Color(0xFF0B0D12),
    onBackground = Color(0xFFE7E9EE),
    surface = Color(0xFF0B0D12),
    onSurface = Color(0xFFE7E9EE),
    surfaceVariant = Color(0xFF1D2232),
    onSurfaceVariant = Color(0xFFB4BACB),
    surfaceContainer = Color(0xFF151925),
    surfaceContainerHigh = Color(0xFF1D2232),
    error = Color(0xFFFCA5A5),
)

private val Light = lightColorScheme(
    primary = Color(0xFFC2410C),
    onPrimary = Color.White,
    primaryContainer = Color(0xFFFFEDD5),
    onPrimaryContainer = Color(0xFF7C2D12),
    secondary = Color(0xFFEA580C),
    background = Color(0xFFFBFAF8),
    surface = Color(0xFFFBFAF8),
    surfaceVariant = Color(0xFFEFEBE6),
    onSurfaceVariant = Color(0xFF55504A),
)

/** The app's own colours; the wallpaper colours of Android 12+ are not used, so the brand stays. */
@Composable
fun MoviesCastTheme(content: @Composable () -> Unit) {
    val dark = isSystemInDarkTheme()
    MaterialTheme(colorScheme = if (dark) Dark else Light, content = content)
}
