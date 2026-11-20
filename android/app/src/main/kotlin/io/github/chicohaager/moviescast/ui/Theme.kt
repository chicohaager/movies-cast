package io.github.chicohaager.moviescast.ui

import android.content.res.Configuration
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.BorderStroke
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.TextButton
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.material3.ButtonDefaults
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.foundation.Image
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
 * The colours: Raumklang's (owner's Sonos app, web/src/styles.css): a calm near-black, warm off-white writing and a soft
 * gold as the one accent. The logo keeps its orange planet (android/art); the wordmark, buttons and focus frames are gold.
 */
object Brand {
    /** Raumklang --accent. */
    val Gold = Color(0xFFDCBF8F)
    val GoldLight = Color(0xFFEBD6AE)
    val GoldDeep = Color(0xFFC4A26E)
    /** Raumklang --accent-ink: writing on gold. */
    val GoldInk = Color(0xFF1B150B)
    /** Raumklang --muted: the quiet second colour (progress, a readable text's frame). */
    val Mist = Color(0xFFA7A49D)
    /** Raumklang --bg. */
    val Night = Color(0xFF0C0D0F)
    /** The gold for the wordmark, focus frames and the main buttons. */
    val Accent = Brush.linearGradient(listOf(GoldLight, Gold, GoldDeep))
}

/** Space Grotesk (SIL OFL 1.1, res/font): headings and the wordmark; Cyrillic and CJK fall back to the system font. */
@OptIn(androidx.compose.ui.text.ExperimentalTextApi::class)
private fun grotesk(weight: Int) = Font(R.font.space_grotesk, FontWeight(weight), variationSettings = FontVariation.Settings(FontVariation.weight(weight)))

val SpaceGrotesk = FontFamily(grotesk(400), grotesk(500), grotesk(600), grotesk(700))

// Raumklang's tokens; the cards there are white at 4.5 / 7.5 / 11 % over --bg, here laid flat.
private val Dark = darkColorScheme(
    primary = Brand.Gold,
    onPrimary = Brand.GoldInk,
    primaryContainer = Color(0xFF3A3122),
    onPrimaryContainer = Color(0xFFF3E6CC),
    secondary = Brand.Mist,
    onSecondary = Color(0xFF1B1A18),
    tertiary = Brand.GoldLight,
    // The screens lie on the sky of [SpaceBackground]: the scaffolds and bars are see-through.
    background = Color.Transparent,
    onBackground = Color(0xFFF3F1EC),
    surface = Color.Transparent,
    onSurface = Color(0xFFF3F1EC),
    surfaceVariant = Color(0xFF272829),
    onSurfaceVariant = Brand.Mist,
    surfaceContainerLowest = Color(0xFF08090A),
    surfaceContainerLow = Color(0xFF121315),
    surfaceContainer = Color(0xFF17181A),
    surfaceContainerHigh = Color(0xFF1E1F21),
    surfaceContainerHighest = Color(0xFF272829),
    outline = Color(0xFF6E6C67),
    outlineVariant = Color(0xFF2E2F31),
    inverseSurface = Color(0xFFF3F1EC),
    inverseOnSurface = Color(0xFF17181A),
    error = Color(0xFFEF8A78),
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

/** Android TV's design guidelines ask for a dark interface. */
@Composable
fun isTv(): Boolean = (LocalConfiguration.current.uiMode and Configuration.UI_MODE_TYPE_MASK) == Configuration.UI_MODE_TYPE_TELEVISION

/**
 * The app's own colours, always dark as Raumklang (owner, 2026-11-20); the wallpaper colours of Android 12+ and the
 * phone's light mode are not used.
 */
@Composable
fun MoviesCastTheme(content: @Composable () -> Unit) {
    val scheme: ColorScheme = Dark
    MaterialTheme(colorScheme = scheme, typography = remember { typography() }, shapes = shapes) {
        Box(Modifier.fillMaxSize()) {
            SpaceBackground()
            // Pages without a Scaffold (the login page) would otherwise write in the default black.
            CompositionLocalProvider(LocalContentColor provides scheme.onBackground) { content() }
        }
    }
}

/** A star: where (0…1), how big (dp) and how bright. */
private class Star(val x: Float, val y: Float, val radius: Float, val alpha: Float)

/**
 * The sky behind every page: Raumklang's near-black with a faint gold glow bottom right and a warm grey one top left,
 * and about 140 stars, always in the same places (a fixed seed), drawn once per size; nothing moves, so it costs
 * nothing while a film list scrolls.
 */
@Composable
fun SpaceBackground() {
    val stars = remember {
        val random = Random(1977)
        List(140) { Star(random.nextFloat(), random.nextFloat(), 0.5f + random.nextFloat() * random.nextFloat() * 1.4f, 0.25f + random.nextFloat() * 0.75f) }
    }
    Canvas(Modifier.fillMaxSize()) {
        val w = size.width
        val h = size.height
        val long = maxOf(w, h)
        drawRect(Brush.verticalGradient(listOf(Color(0xFF141518), Brand.Night, Color(0xFF08090A))))
        drawRect(Brush.radialGradient(listOf(Brand.Gold.copy(alpha = 0.16f), Color.Transparent), center = Offset(w * 0.95f, h * 1.02f), radius = long * 0.6f))
        drawRect(Brush.radialGradient(listOf(Brand.Mist.copy(alpha = 0.10f), Color.Transparent), center = Offset(w * 0.02f, 0f), radius = long * 0.5f))
        for (s in stars) drawCircle(Color.White.copy(alpha = s.alpha), radius = s.radius * density, center = Offset(s.x * w, s.y * h))
    }
}

/** The logo: the planet with its orbit, and "Movies" over "CAST" in gold. Read out as the app's name. */
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
            Text("CAST", style = TextStyle(fontFamily = SpaceGrotesk, fontWeight = FontWeight.Medium, fontSize = textSize.sp, letterSpacing = (textSize * 0.16f).sp, brush = Brand.Accent))
        }
    }
}

/** How much a button in focus grows: as the library's tiles, so the remote's place is seen from the sofa. */
private const val FOCUS_SCALE = 1.08f

/** A wide element (a row of the "Home screen" page) grows by no more than this, so it does not cover its neighbours. */
private val FOCUS_GROWTH_MAX = 16.dp

/**
 * Grows the element in focus and lifts it with a shadow, as Infuse and the Leanback cards of Android TV do: a change of
 * colour alone was hard to tell from the sofa (film page, Android TV emulator, 2026-11-14).
 */
@Composable
private fun Modifier.lifted(focused: Boolean, shape: Shape): Modifier {
    var width by remember { mutableStateOf(0) }
    val most = with(LocalDensity.current) { FOCUS_GROWTH_MAX.toPx() }
    val target = if (width > 0) minOf(FOCUS_SCALE, 1f + most / width) else FOCUS_SCALE
    val scale by animateFloatAsState(if (focused) target else 1f, label = "focus")
    return onSizeChanged { width = it.width }.graphicsLayer {
        scaleX = scale
        scaleY = scale
        shadowElevation = if (focused) 12.dp.toPx() else 0f
        this.shape = shape
        clip = false
    }
}

/**
 * The main button of a page (play, log in): filled with the gold gradient. In focus it grows and gets a white
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
            contentColor = Brand.GoldInk,
            disabledContainerColor = Color.Transparent,
            disabledContentColor = MaterialTheme.colorScheme.onSurface.copy(alpha = 0.45f),
        ),
        modifier = modifier
            .onFocusChanged { focused = it.isFocused }
            .lifted(focused, CircleShape)
            .then(ring)
            .background(if (enabled) Brand.Accent else SolidColor(MaterialTheme.colorScheme.onSurface.copy(alpha = 0.12f)), CircleShape),
        content = content,
    )
}

/**
 * Every other button of the app (in place of Material's OutlinedButton). In focus it grows, is lifted with a shadow,
 * fills and gets a gold frame, like the library's tiles; Material's own focus shade on an outlined
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
        border = if (focused) BorderStroke(2.5.dp, Brand.Accent) else ButtonDefaults.outlinedButtonBorder(enabled),
        colors = ButtonDefaults.outlinedButtonColors(
            containerColor = if (focused) MaterialTheme.colorScheme.surfaceContainerHighest else Color.Transparent,
        ),
        modifier = modifier.onFocusChanged { focused = it.isFocused }.lifted(focused, shape),
        content = content,
    )
}

/**
 * Every text button of the app (in place of Material's TextButton: Back, ⋮, reload, language, About, the dialogs'
 * buttons). Out of focus it is plain text as before; in focus it fills and gets the gold frame of [SoftButton]:
 * Material's own focus shade on a text button was hard to see on the Google TV Streamer (owner, 2026-11-16).
 */
@Composable
fun SoftTextButton(
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    content: @Composable RowScope.() -> Unit,
) {
    var focused by remember { mutableStateOf(false) }
    TextButton(
        onClick = onClick,
        enabled = enabled,
        shape = CircleShape,
        border = if (focused) BorderStroke(2.5.dp, Brand.Accent) else null,
        colors = ButtonDefaults.textButtonColors(
            containerColor = if (focused) MaterialTheme.colorScheme.surfaceContainerHighest else Color.Transparent,
        ),
        modifier = modifier.onFocusChanged { focused = it.isFocused }.lifted(focused, CircleShape),
        content = content,
    )
}

/**
 * Every entry of a drop-down menu (in place of Material's DropdownMenuItem): in focus it is filled and framed in
 * gold as [SoftTextButton]; Material's focus shade was a faint lighter stripe on the Streamer (2026-11-16).
 */
@Composable
fun SoftMenuItem(
    text: @Composable () -> Unit,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
) {
    var focused by remember { mutableStateOf(false) }
    DropdownMenuItem(
        text = text,
        onClick = onClick,
        modifier = modifier
            .onFocusChanged { focused = it.isFocused }
            .padding(horizontal = 4.dp)
            .background(if (focused) MaterialTheme.colorScheme.surfaceContainerHighest else Color.Transparent, RoundedCornerShape(10.dp))
            .border(2.5.dp, if (focused) Brand.Accent else SolidColor(Color.Transparent), RoundedCornerShape(10.dp)),
    )
}

/**
 * The night colours for a page that lies on a film's backdrop: the picture is dark under its veil, so the writing
 * must be light (the whole app is dark since 2026-11-20; kept so a backdrop page stays dark whatever the theme).
 */
@Composable
fun OnBackdrop(content: @Composable () -> Unit) {
    MaterialTheme(colorScheme = Dark, typography = MaterialTheme.typography, shapes = MaterialTheme.shapes) {
        CompositionLocalProvider(LocalContentColor provides Dark.onBackground) { content() }
    }
}

/**
 * A piece of text a remote can stop on (a film's overview, its file facts): it takes the focus, so ↓ reaches it and
 * the page scrolls to it, and shows a faint frame in Raumklang's warm grey while it has it. A phone does not focus it.
 */
@Composable
fun Modifier.readable(): Modifier {
    var focused by remember { mutableStateOf(false) }
    return onFocusChanged { focused = it.isFocused }
        .border(2.dp, if (focused) Brand.Mist.copy(alpha = 0.8f) else Color.Transparent, RoundedCornerShape(10.dp))
        .padding(6.dp)
        .focusable()
}
