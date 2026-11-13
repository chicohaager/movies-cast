package io.github.chicohaager.moviescast.ui

import android.content.res.Configuration
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.foundation.Image
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.requiredSize
import androidx.compose.material3.ColorScheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Text
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.material3.LocalContentColor
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontVariation
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.foundation.shape.RoundedCornerShape
import io.github.chicohaager.moviescast.R
import kotlin.random.Random

/**
 * The look: deep space with a nebula, the logo's planet gradient (amber → rose → violet) and its orbit's cyan.
 * The masters of logo and banner are in android/art; their colours are these.
 */
object Brand {
    val Amber = Color(0xFFFFC56B)
    val Rose = Color(0xFFFB6F92)
    val Violet = Color(0xFF8B5CF6)
    val Cyan = Color(0xFF22D3EE)
    val Ice = Color(0xFFE0F2FE)
    val Night = Color(0xFF07071A)
    /** The planet of the logo, for the wordmark, focus frames and progress. */
    val Planet = Brush.linearGradient(listOf(Amber, Rose, Violet))
    val Orbit = Brush.linearGradient(listOf(Cyan, Violet))
}

/** Space Grotesk (SIL OFL 1.1, res/font): headings and the wordmark; Cyrillic and CJK fall back to the system font. */
@OptIn(androidx.compose.ui.text.ExperimentalTextApi::class)
private fun grotesk(weight: Int) = Font(R.font.space_grotesk, FontWeight(weight), variationSettings = FontVariation.Settings(FontVariation.weight(weight)))

val SpaceGrotesk = FontFamily(grotesk(400), grotesk(500), grotesk(600), grotesk(700))

private val Dark = darkColorScheme(
    primary = Color(0xFFFF8FAB),
    onPrimary = Color(0xFF2A0614),
    primaryContainer = Color(0xFF5B1B47),
    onPrimaryContainer = Color(0xFFFFD9E3),
    secondary = Color(0xFF67E8F9),
    onSecondary = Color(0xFF00222A),
    tertiary = Brand.Amber,
    // The screens lie on the starry sky of [SpaceBackground]: the scaffolds and bars are see-through.
    background = Color.Transparent,
    onBackground = Color(0xFFEDEBFF),
    surface = Color.Transparent,
    onSurface = Color(0xFFEDEBFF),
    surfaceVariant = Color(0xFF1E1B45),
    onSurfaceVariant = Color(0xFFB8B4E0),
    surfaceContainerLowest = Color(0xFF07071A),
    surfaceContainerLow = Color(0xFF0E0D27),
    surfaceContainer = Color(0xFF15133A),
    surfaceContainerHigh = Color(0xFF1C1946),
    surfaceContainerHighest = Color(0xFF242054),
    outline = Color(0xFF4A4590),
    outlineVariant = Color(0xFF2E2A63),
    inverseSurface = Color(0xFFEDEBFF),
    inverseOnSurface = Color(0xFF15133A),
    error = Color(0xFFFCA5A5),
)

private val Light = lightColorScheme(
    primary = Color(0xFFB4235A),
    onPrimary = Color.White,
    primaryContainer = Color(0xFFFFD9E3),
    onPrimaryContainer = Color(0xFF3E0420),
    secondary = Color(0xFF0E7490),
    onSecondary = Color.White,
    tertiary = Color(0xFFB45309),
    background = Color.Transparent,
    onBackground = Color(0xFF16133A),
    surface = Color.Transparent,
    onSurface = Color(0xFF16133A),
    surfaceVariant = Color(0xFFE9E5FB),
    onSurfaceVariant = Color(0xFF4A4672),
    surfaceContainerLowest = Color.White,
    surfaceContainerLow = Color(0xFFFBFAFF),
    surfaceContainer = Color(0xFFF4F1FF),
    surfaceContainerHigh = Color(0xFFEEEAFE),
    surfaceContainerHighest = Color(0xFFE7E2FC),
    outline = Color(0xFF8A84B8),
    outlineVariant = Color(0xFFD3CEF0),
)

private fun typography(): Typography {
    val base = Typography()
    fun TextStyle.grotesk(weight: Int, spacing: Float = 0f) = copy(fontFamily = SpaceGrotesk, fontWeight = FontWeight(weight), letterSpacing = spacing.sp)
    return base.copy(
        displayLarge = base.displayLarge.grotesk(700, -1f),
        displayMedium = base.displayMedium.grotesk(700, -0.5f),
        displaySmall = base.displaySmall.grotesk(700),
        headlineLarge = base.headlineLarge.grotesk(700),
        headlineMedium = base.headlineMedium.grotesk(700),
        headlineSmall = base.headlineSmall.grotesk(600),
        titleLarge = base.titleLarge.grotesk(600),
        titleMedium = base.titleMedium.grotesk(600, 0.2f),
    )
}

private val shapes = Shapes(
    extraSmall = RoundedCornerShape(8.dp),
    small = RoundedCornerShape(12.dp),
    medium = RoundedCornerShape(16.dp),
    large = RoundedCornerShape(24.dp),
)

/** Android TV's design guidelines ask for a dark interface; a TV therefore always gets the night sky. */
@Composable
fun isTv(): Boolean = (LocalConfiguration.current.uiMode and Configuration.UI_MODE_TYPE_MASK) == Configuration.UI_MODE_TYPE_TELEVISION

/** The app's own colours; the wallpaper colours of Android 12+ are not used, so the brand stays. */
@Composable
fun MoviesCastTheme(content: @Composable () -> Unit) {
    val dark = isSystemInDarkTheme() || isTv()
    val scheme: ColorScheme = if (dark) Dark else Light
    MaterialTheme(colorScheme = scheme, typography = remember { typography() }, shapes = shapes) {
        Box(Modifier.fillMaxSize()) {
            SpaceBackground(dark)
            // Pages without a Scaffold (the login page) would otherwise write in the default black.
            CompositionLocalProvider(LocalContentColor provides scheme.onBackground) { content() }
        }
    }
}

/** A star: where (0…1), how big (dp) and how bright. */
private class Star(val x: Float, val y: Float, val radius: Float, val alpha: Float)

/**
 * The sky behind every page: a deep blue-violet night with two nebulae (rose bottom right, cyan top left) and
 * about 140 stars, always in the same places (a fixed seed), drawn once per size; nothing moves, so it costs
 * nothing while a film list scrolls. By day (a phone in light mode) the same sky at dawn.
 */
@Composable
fun SpaceBackground(dark: Boolean) {
    val stars = remember {
        val random = Random(1977)
        List(140) { Star(random.nextFloat(), random.nextFloat(), 0.5f + random.nextFloat() * random.nextFloat() * 1.4f, 0.25f + random.nextFloat() * 0.75f) }
    }
    Canvas(Modifier.fillMaxSize()) {
        val w = size.width
        val h = size.height
        val long = maxOf(w, h)
        if (dark) {
            drawRect(Brush.verticalGradient(listOf(Color(0xFF0D0B2B), Brand.Night, Color(0xFF05050F))))
            drawRect(Brush.radialGradient(listOf(Color(0x66C026D3), Color.Transparent), center = Offset(w * 0.95f, h * 1.02f), radius = long * 0.6f))
            drawRect(Brush.radialGradient(listOf(Color(0x3322D3EE), Color.Transparent), center = Offset(w * 0.02f, 0f), radius = long * 0.5f))
            drawRect(Brush.radialGradient(listOf(Color(0x332B1C66), Color.Transparent), center = Offset(w * 0.5f, h * 0.35f), radius = long * 0.55f))
        } else {
            drawRect(Brush.verticalGradient(listOf(Color(0xFFF3F0FF), Color(0xFFFBFAFF), Color(0xFFFFF1F5))))
            drawRect(Brush.radialGradient(listOf(Color(0x33FB6F92), Color.Transparent), center = Offset(w * 0.95f, h * 1.02f), radius = long * 0.6f))
            drawRect(Brush.radialGradient(listOf(Color(0x2222D3EE), Color.Transparent), center = Offset(w * 0.02f, 0f), radius = long * 0.5f))
        }
        val starColor = if (dark) Color.White else Color(0xFF8B5CF6)
        val scale = if (dark) 1f else 0.35f
        for (s in stars) drawCircle(starColor.copy(alpha = s.alpha * scale), radius = s.radius * density, center = Offset(s.x * w, s.y * h))
    }
}

/** The logo: the planet with its orbit, and "Movies" over "CAST" in the planet's colours. Read out as the app's name. */
@Composable
fun Wordmark(name: String, modifier: Modifier = Modifier, markSize: Dp = 44.dp, textSize: Int = 20) {
    Row(
        modifier.clearAndSetSemantics { contentDescription = name },
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        // The foreground of the launcher icon has the mark in its middle 60 % (the rest is the launcher's mask margin):
        // drawn 1.7 times as large and left to overflow the box, so the mark itself fills [markSize].
        Box(Modifier.size(markSize), contentAlignment = Alignment.Center) {
            Image(painterResource(R.drawable.ic_launcher_foreground), contentDescription = null, modifier = Modifier.requiredSize(markSize * 1.7f))
        }
        Column(verticalArrangement = Arrangement.spacedBy((-textSize * 0.18f).dp)) {
            Text("Movies", style = TextStyle(fontFamily = SpaceGrotesk, fontWeight = FontWeight.Bold, fontSize = textSize.sp, letterSpacing = (-0.3).sp, color = MaterialTheme.colorScheme.onBackground))
            Text("CAST", style = TextStyle(fontFamily = SpaceGrotesk, fontWeight = FontWeight.Medium, fontSize = textSize.sp, letterSpacing = (textSize * 0.16f).sp, brush = Brand.Planet))
        }
    }
}

/**
 * The main button of a page (play, log in): filled with the planet's gradient. On a TV the button in focus gets a
 * white ring, as Material's own focus shade is hard to see from the sofa.
 */
@Composable
fun GlowButton(onClick: () -> Unit, modifier: Modifier = Modifier, enabled: Boolean = true, content: @Composable RowScope.() -> Unit) {
    var focused by remember { mutableStateOf(false) }
    val ring = if (focused) Modifier.border(2.5.dp, Color.White, CircleShape) else Modifier
    Button(
        onClick = onClick,
        enabled = enabled,
        colors = ButtonDefaults.buttonColors(
            containerColor = Color.Transparent,
            contentColor = Color(0xFF160B2E),
            disabledContainerColor = Color.Transparent,
            disabledContentColor = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.45f),
        ),
        modifier = modifier
            .onFocusChanged { focused = it.isFocused }
            .then(ring)
            .background(if (enabled) Brand.Planet else SolidColor(MaterialTheme.colorScheme.onSurface.copy(alpha = 0.12f)), CircleShape),
        content = content,
    )
}
