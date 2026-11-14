package io.github.chicohaager.moviescast.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.Alignment
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ExperimentalLayoutApi
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.gestures.BringIntoViewSpec
import androidx.compose.foundation.gestures.LocalBringIntoViewSpec
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import coil3.compose.AsyncImage
import io.github.chicohaager.moviescast.MoviesCastApp
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import io.github.chicohaager.moviescast.R
import io.github.chicohaager.moviescast.UiState
import io.github.chicohaager.moviescast.core.MediaItem
import io.github.chicohaager.moviescast.core.Track
import io.github.chicohaager.moviescast.core.episodeLine

/**
 * On a TV Compose (foundation 1.12.1) moves the element in focus to 30 % of the page's height (PivotBringIntoViewSpec,
 * chosen when the device has android.software.leanback); on the film's page that pushed the logo half out at the top
 * as soon as the main button took the focus (Android TV emulator, 2026-11-14). The page scrolls only as far as needed.
 */
@OptIn(ExperimentalFoundationApi::class)
private val JustEnough = object : BringIntoViewSpec {}

@OptIn(ExperimentalMaterial3Api::class, ExperimentalFoundationApi::class)
@Composable
fun DetailScreen(
    state: UiState,
    onBack: () -> Unit,
    onOpen: (MediaItem) -> Unit,
    onPlay: (MediaItem, Long) -> Unit,
    onPlayed: (MediaItem, Boolean) -> Unit,
    onFavorite: (MediaItem, Boolean) -> Unit,
) {
    CompositionLocalProvider(LocalBringIntoViewSpec provides JustEnough) {
        DetailPage(state, onBack, onOpen, onPlay, onPlayed, onFavorite)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DetailPage(
    state: UiState,
    onBack: () -> Unit,
    onOpen: (MediaItem) -> Unit,
    onPlay: (MediaItem, Long) -> Unit,
    onPlayed: (MediaItem, Boolean) -> Unit,
    onFavorite: (MediaItem, Boolean) -> Unit,
) {
    val item = state.detail ?: return
    val app = LocalContext.current.applicationContext as MoviesCastApp
    val session = app.session
    val backdrop = session?.let { app.client.backdropUrl(it, item, 1920) }
    val logo = session?.let { app.client.logoUrl(it, item, 800) }
    if (backdrop != null) {
        OnBackdrop { BackdropPage(state, item, backdrop, logo, onBack, onOpen, onPlay, onPlayed, onFavorite) }
        return
    }
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(item.name, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                navigationIcon = { TextButton(onClick = onBack, modifier = Modifier.testTag("back")) { Text("‹ " + stringResource(R.string.back)) } },
            )
        },
    ) { padding ->
        BoxWithConstraints(Modifier.padding(padding).fillMaxSize()) {
            val wide = maxWidth >= 600.dp
            if (wide) {
                Row(Modifier.fillMaxSize().padding(horizontal = 24.dp), horizontalArrangement = Arrangement.spacedBy(24.dp)) {
                    GlowingPoster(item, Modifier.width(260.dp).padding(top = 8.dp))
                    Column(Modifier.weight(1f).verticalScroll(rememberScrollState()).padding(bottom = 24.dp)) {
                        Facts(state, item, onOpen, onPlay, onPlayed, onFavorite)
                    }
                }
            } else {
                Column(
                    Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 24.dp),
                    verticalArrangement = Arrangement.spacedBy(16.dp),
                ) {
                    GlowingPoster(item, Modifier.width(180.dp))
                    Facts(state, item, onOpen, onPlay, onPlayed, onFavorite)
                }
            }
        }
    }
}

/**
 * The film's page over its backdrop, as Infuse shows it (the owner's yardstick, ROADMAP A15): the picture fills the
 * screen, a veil of the night sky from the left and from below keeps the writing readable, the title is the film's
 * logo when the server has one. On a phone the picture is a band at the top that runs into the night below.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun BackdropPage(
    state: UiState,
    item: MediaItem,
    backdrop: String,
    logo: String?,
    onBack: () -> Unit,
    onOpen: (MediaItem) -> Unit,
    onPlay: (MediaItem, Long) -> Unit,
    onPlayed: (MediaItem, Boolean) -> Unit,
    onFavorite: (MediaItem, Boolean) -> Unit,
) {
    BoxWithConstraints(Modifier.fillMaxSize().background(Brand.Night)) {
        val wide = maxWidth >= 600.dp
        val band = maxWidth * 9 / 16
        AsyncImage(
            model = backdrop,
            contentDescription = null,
            contentScale = ContentScale.Crop,
            alignment = Alignment.TopCenter,
            modifier = (if (wide) Modifier.fillMaxSize() else Modifier.fillMaxWidth().height(band)).testTag("backdrop"),
        )
        if (wide) {
            Box(Modifier.fillMaxSize().background(Brush.horizontalGradient(0f to Brand.Night.copy(alpha = 0.94f), 0.45f to Brand.Night.copy(alpha = 0.7f), 0.75f to Color.Transparent)))
            Box(Modifier.fillMaxSize().background(Brush.verticalGradient(0f to Brand.Night.copy(alpha = 0.55f), 0.18f to Color.Transparent, 0.6f to Color.Transparent, 1f to Brand.Night.copy(alpha = 0.92f))))
        } else {
            Box(Modifier.fillMaxWidth().height(band).background(Brush.verticalGradient(0f to Brand.Night.copy(alpha = 0.5f), 0.3f to Color.Transparent, 1f to Brand.Night)))
        }
        Scaffold(
            containerColor = Color.Transparent,
            topBar = {
                TopAppBar(
                    title = {},
                    colors = TopAppBarDefaults.topAppBarColors(containerColor = Color.Transparent),
                    navigationIcon = { TextButton(onClick = onBack, modifier = Modifier.testTag("back")) { Text("‹ " + stringResource(R.string.back)) } },
                )
            },
        ) { padding ->
            if (wide) {
                Column(
                    Modifier.padding(padding).fillMaxHeight().fillMaxWidth(0.55f).verticalScroll(rememberScrollState())
                        .padding(start = 48.dp, end = 24.dp, top = 24.dp, bottom = 32.dp),
                ) {
                    Facts(state, item, onOpen, onPlay, onPlayed, onFavorite, logo)
                }
            } else {
                Column(Modifier.padding(padding).fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 24.dp)) {
                    Spacer(Modifier.height((band - 120.dp).coerceAtLeast(0.dp)))
                    Facts(state, item, onOpen, onPlay, onPlayed, onFavorite, logo)
                }
            }
        }
    }
}

/** The title: the film's logo when the server has one and it loads, else the name in text. */
@Composable
private fun Title(item: MediaItem, logo: String?) {
    var failed by remember(logo) { mutableStateOf(false) }
    if (logo != null && !failed) {
        AsyncImage(
            model = logo,
            contentDescription = item.name,
            contentScale = ContentScale.Fit,
            alignment = Alignment.CenterStart,
            onError = { failed = true },
            // a fixed height: the page must not grow when the logo arrives after the main button took the focus
            modifier = Modifier.height(120.dp).widthIn(max = 460.dp).fillMaxWidth().testTag("logo"),
        )
    } else {
        Text(item.name, style = MaterialTheme.typography.headlineMedium)
    }
}

@OptIn(ExperimentalLayoutApi::class)
@Composable
private fun Facts(
    state: UiState,
    item: MediaItem,
    onOpen: (MediaItem) -> Unit,
    onPlay: (MediaItem, Long) -> Unit,
    onPlayed: (MediaItem, Boolean) -> Unit,
    onFavorite: (MediaItem, Boolean) -> Unit,
    logo: String? = null,
) {
    Column(verticalArrangement = Arrangement.spacedBy(14.dp)) {
        Title(item, logo)
        val line = listOfNotNull(
            item.seriesName.takeIf { it.isNotEmpty() },
            item.year.takeIf { it > 0 }?.toString(),
            (item.runtimeSeconds / 60).takeIf { it > 0 }?.let { stringResource(R.string.detail_minutes, it.toInt()) },
            item.rating.takeIf { it.isNotEmpty() },
            item.genres.take(3).joinToString(", ").takeIf { it.isNotEmpty() },
        ).joinToString(" · ")
        if (line.isNotEmpty()) Text(line, color = MaterialTheme.colorScheme.onSurfaceVariant, style = MaterialTheme.typography.bodyMedium)

        if (item.playable) {
            // With a remote control the first OK should play: the main button takes the focus
            // when the page opens (measured 2026-11-12 on Android TV: nothing was in focus).
            val mainButton = remember { FocusRequester() }
            LaunchedEffect(item.id) { runCatching { mainButton.requestFocus() } }
            FlowRow(horizontalArrangement = Arrangement.spacedBy(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                if (item.resumable) {
                    GlowButton(onClick = { onPlay(item, item.resumeSeconds) }, modifier = Modifier.focusRequester(mainButton).testTag("resume")) {
                        Text(stringResource(R.string.play_resume, clock(item.resumeSeconds)))
                    }
                    SoftButton(onClick = { onPlay(item, 0) }, modifier = Modifier.testTag("restart")) {
                        Text(stringResource(R.string.play_restart))
                    }
                } else {
                    GlowButton(onClick = { onPlay(item, 0) }, modifier = Modifier.focusRequester(mainButton).testTag("play")) { Text("▶  " + stringResource(R.string.play), fontWeight = FontWeight.Bold) }
                }
                SoftButton(onClick = { onPlayed(item, !item.played) }, modifier = Modifier.testTag("played")) {
                    Text(if (item.played) "✓  " + stringResource(R.string.mark_unwatched) else stringResource(R.string.mark_watched))
                }
            }
        }
        // A series can be a favourite too, so this button does not depend on "playable".
        SoftButton(onClick = { onFavorite(item, !item.favorite) }, modifier = Modifier.testTag("favourite")) {
            Text(if (item.favorite) "♥  " + stringResource(R.string.favourite_remove) else "♡  " + stringResource(R.string.favourite_add))
        }

        if (item.overview.isNotEmpty()) Text(item.overview, style = MaterialTheme.typography.bodyLarge)

        if (item.playable) {
            val file = listOfNotNull(
                item.container.takeIf { it.isNotEmpty() }?.uppercase(),
                item.videoCodec.takeIf { it.isNotEmpty() }?.uppercase(),
                if (item.width > 0 && item.height > 0) "${item.width}×${item.height}" else null,
                item.sizeBytes.takeIf { it > 0 }?.let { fileSize(it) },
            ).joinToString(" · ")
            Fact(stringResource(R.string.detail_file), file)
            Fact(stringResource(R.string.detail_audio), item.audio.joinToString("\n") { trackLine(it) })
            Fact(stringResource(R.string.detail_subtitles), item.subtitles.joinToString(", ") { it.title.ifEmpty { it.codec } })
        }

        if (state.episodes.isNotEmpty()) {
            Text(stringResource(R.string.episodes), style = MaterialTheme.typography.titleLarge)
            Column {
                var season = -1
                for (episode in state.episodes) {
                    if (episode.season != season) {
                        season = episode.season
                        Text(
                            if (season == 0) stringResource(R.string.season_specials) else stringResource(R.string.season_label, season),
                            style = MaterialTheme.typography.titleSmall,
                            color = MaterialTheme.colorScheme.primary,
                            modifier = Modifier.padding(top = 18.dp, bottom = 6.dp).testTag("season"),
                        )
                    }
                    HorizontalDivider()
                    Text(
                        (if (episode.played) "✓  " else "") +
                            episodeLine(stringResource(R.string.episode_label, episode.season, episode.episode, episode.name), episode.name, episode.seriesName),
                        modifier = Modifier.fillMaxWidth().clickable { onOpen(episode) }.padding(vertical = 14.dp).testTag("episode"),
                    )
                }
            }
        }
    }
}

/** 2.3 GB, 740 MB. */
internal fun fileSize(bytes: Long): String =
    if (bytes >= 1_000_000_000) "%.1f GB".format(bytes / 1e9) else "%d MB".format((bytes / 1_000_000).coerceAtLeast(1))

private fun trackLine(track: Track): String {
    val title = track.title.ifEmpty { track.language }
    val codec = track.codec.uppercase()
    return when {
        title.isEmpty() -> codec
        title.contains(codec, ignoreCase = true) -> title
        else -> "$title ($codec)"
    }
}

/** The poster on the film's page, with a halo in the planet's orange behind it. */
@Composable
private fun GlowingPoster(item: MediaItem, modifier: Modifier) {
    Box(modifier, contentAlignment = Alignment.Center) {
        Box(
            Modifier.matchParentSize().padding(4.dp)
                .background(Brush.radialGradient(listOf(Brand.Orange.copy(alpha = 0.35f), Brand.Sky.copy(alpha = 0.12f), Color.Transparent)), RoundedCornerShape(24.dp)),
        )
        Poster(
            item,
            Modifier.fillMaxWidth().padding(10.dp)
                .border(1.dp, Brush.linearGradient(listOf(Color.White.copy(alpha = 0.35f), Color.White.copy(alpha = 0.05f))), RoundedCornerShape(12.dp)),
            widthPx = 600,
        )
    }
}

@Composable
private fun Fact(label: String, value: String) {
    if (value.isEmpty()) return
    Column {
        Text(label, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Text(value, style = MaterialTheme.typography.bodyMedium)
    }
}
