package io.github.chicohaager.moviescast.ui

import android.content.res.Configuration
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.BorderStroke
import androidx.compose.material3.Button
import androidx.compose.material3.OutlinedButton
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.graphicsLayer
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
 * The look: a deep-sea blue night like the sky over the earth, the logo's planet as a warm sun in orange (movies-cast's
 * colour from the start) and its orbit in sky blue. The masters of logo and banner are in android/art; their colours are these.
 */
object Brand {
    val Light = Color(0xFFFFD27A)
    val Orange = Color(0xFFFB923C)
    val Ember = Color(0xFFEA580C)
    val Sky = Color(0xFF38BDF8)
    val Night = Color(0xFF060D1F)
    /** The planet of the logo, for the wordmark, focus frames and the main buttons. */
    val Planet = Brush.linearGradient(listOf(Light, Orange, Ember))
}

/** Space Grotesk (SIL OFL 1.1, res/font): headings and the wordmark; Cyrillic and CJK fall back to the system font. */
@OptIn(androidx.compose.ui.text.ExperimentalTextApi::class)
private fun grotesk(weight: Int) = Font(R.font.space_grotesk, FontWeight(weight), variationSettings = FontVariation.Settings(FontVariation.weight(weight)))

val SpaceGrotesk = FontFamily(grotesk(400), grotesk(500), grotesk(600), grotesk(700))

private val Dark = darkColorScheme(
    primary = Color(0xFFFDBA74),
    onPrimary = Color(0xFF3A1A04),
    primaryContainer = Color(0xFF7C2D12),
    onPrimaryContainer = Color(0xFFFFEDD5),
    secondary = Color(0xFF7DD3FC),
    onSecondary = Color(0xFF062033),
    tertiary = Brand.Light,
    // The screens lie on the sky of [SpaceBackground]: the scaffolds and bars are see-through.
    background = Color.Transparent,
    onBackground = Color(0xFFEAF2FF),
    surface = Color.Transparent,
    onSurface = Color(0xFFEAF2FF),
    surfaceVariant = Color(0xFF16264A),
    onSurfaceVariant = Color(0xFFA9B8D6),
    surfaceContainerLowest = Color(0xFF03060F),
    surfaceContainerLow = Color(0xFF0A1430),
    surfaceContainer = Color(0xFF0F1B3B),
    surfaceContainerHigh = Color(0xFF152345),
    surfaceContainerHighest = Color(0xFF1B2B52),
    outline = Color(0xFF3B5584),
    outlineVariant = Color(0xFF26406E),
    inverseSurface = Color(0xFFEAF2FF),
    inverseOnSurface = Color(0xFF0F1B3B),
    error = Color(0xFFFCA5A5),
)

private val Light = lightColorScheme(
    primary = Color(0xFFC2410C),
    onPrimary = Color.White,
    primaryContainer = Color(0xFFFFEDD5),
    onPrimaryContainer = Color(0xFF7C2D12),
    secondary = Color(0xFF0369A1),
    onSecondary = Color.White,
    tertiary = Color(0xFFB45309),
    background = Color.Transparent,
    onBackground = Color(0xFF0F1A33),
    surface = Color.Transparent,
    onSurface = Color(0xFF0F1A33),
    surfaceVariant = Color(0xFFE6EDF8),
    onSurfaceVariant = Color(0xFF475569),
    surfaceContainerLowest = Color.White,
    surfaceContainerLow = Color(0xFFFAFCFF),
    surfaceContainer = Color(0xFFF1F5FB),
    surfaceContainerHigh = Color(0xFFEAF0F8),
    surfaceContainerHighest = Color(0xFFE2E9F4),
    outline = Color(0xFF94A3B8),
    outlineVariant = Color(0xFFD5DEEB),
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
 * The sky behind every page: a deep-sea blue night with an orange glow bottom right and a sky-blue one top left, and
 * about 140 stars, always in the same places (a fixed seed), drawn once per size; nothing moves, so it costs
 * nothing while a film list scrolls. By day (a phone in light mode) a pale morning sky with faint stars.
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
            drawRect(Brush.verticalGradient(listOf(Color(0xFF0C1A38), Brand.Night, Color(0xFF03060F))))
            drawRect(Brush.radialGradient(listOf(Color(0x38F97316), Color.Transparent), center = Offset(w * 0.95f, h * 1.02f), radius = long * 0.6f))
            drawRect(Brush.radialGradient(listOf(Color(0x3838BDF8), Color.Transparent), center = Offset(w * 0.02f, 0f), radius = long * 0.5f))
        } else {
            drawRect(Brush.verticalGradient(listOf(Color(0xFFF2F7FF), Color(0xFFFFFFFF), Color(0xFFFFF4EA))))
            drawRect(Brush.radialGradient(listOf(Color(0x26F97316), Color.Transparent), center = Offset(w * 0.95f, h * 1.02f), radius = long * 0.6f))
            drawRect(Brush.radialGradient(listOf(Color(0x1A38BDF8), Color.Transparent), center = Offset(w * 0.02f, 0f), radius = long * 0.5f))
        }
        val starColor = if (dark) Color.White else Color(0xFF0369A1)
        val scale = if (dark) 1f else 0.35f
        for (s in stars) drawCircle(starColor.copy(alpha = s.alpha * scale), radius = s.radius * density, center = Offset(s.x * w, s.y * h))
    }
}

/** The logo: the planet with its orbit, and "Movies" over "CAST" in the planet's orange. Read out as the app's name. */
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

/** How much a button in focus grows: as the library's tiles, so the remote's place is seen from the sofa. */
private const val FOCUS_SCALE = 1.08f

/**
 * Grows the element in focus and lifts it with a shadow, as Infuse and the Leanback cards of Android TV do: a change of
 * colour alone was hard to tell from the sofa (film page, Android TV emulator, 2026-11-14).
 */
@Composable
private fun Modifier.lifted(focused: Boolean, shape: Shape): Modifier {
    val scale by animateFloatAsState(if (focused) FOCUS_SCALE else 1f, label = "focus")
    return graphicsLayer {
        scaleX = scale
        scaleY = scale
        shadowElevation = if (focused) 12.dp.toPx() else 0f
        this.shape = shape
        clip = false
    }
}

/**
 * The main button of a page (play, log in): filled with the planet's gradient. In focus it grows and gets a white
 * ring, as Material's own focus shade is hard to see from the sofa.
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
            contentColor = Color(0xFF2A1203),
            disabledContainerColor = Color.Transparent,
            disabledContentColor = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.45f),
        ),
        modifier = modifier
            .onFocusChanged { focused = it.isFocused }
            .lifted(focused, CircleShape)
            .then(ring)
            .background(if (enabled) Brand.Planet else SolidColor(MaterialTheme.colorScheme.onSurface.copy(alpha = 0.12f)), CircleShape),
        content = content,
    )
}

/**
 * Every other button of the app (in place of Material's OutlinedButton). In focus it grows, is lifted with a shadow,
 * fills and gets a frame in the planet's orange, like the library's tiles; Material's own focus shade on an outlined
 * button only turned it a little lighter, hard to tell from the sofa (film page, Android TV emulator, 2026-11-14).
 */
@Composable
fun SoftButton(
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    shape: Shape = CircleShape,
    content: @Composable RowScope.() -> Unit,
) {
    var focused by remember { mutableStateOf(false) }
    OutlinedButton(
        onClick = onClick,
        enabled = enabled,
        shape = shape,
        border = if (focused) BorderStroke(2.5.dp, Brand.Planet) else ButtonDefaults.outlinedButtonBorder(enabled),
        colors = ButtonDefaults.outlinedButtonColors(
            containerColor = if (focused) MaterialTheme.colorScheme.surfaceContainerHighest else Color.Transparent,
        ),
        modifier = modifier.onFocusChanged { focused = it.isFocused }.lifted(focused, shape),
        content = content,
    )
}

/**
 * The night colours for a page that lies on a film's backdrop: the picture is dark under its veil, so the writing
 * must be light even on a phone in light mode.
 */
@Composable
fun OnBackdrop(content: @Composable () -> Unit) {
    MaterialTheme(colorScheme = Dark, typography = MaterialTheme.typography, shapes = MaterialTheme.shapes) {
        CompositionLocalProvider(LocalContentColor provides Dark.onBackground) { content() }
    }
}
