package io.github.chicohaager.moviescast.ui

import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.mikepenz.aboutlibraries.Libs
import com.mikepenz.aboutlibraries.entity.Library
import com.mikepenz.aboutlibraries.util.withJson
import io.github.chicohaager.moviescast.BuildConfig
import io.github.chicohaager.moviescast.R

const val SOURCE_URL = "https://github.com/chicohaager/movies-cast"
const val PRIVACY_URL = "https://github.com/chicohaager/movies-cast/blob/master/PRIVACY.md"

/** FFmpeg is the one library under a copyleft licence; it heads the list so nobody has to look for it. */
private const val FFMPEG = "org.ffmpeg:ffmpeg"

/** Every library of the build with its licence, from the file the AboutLibraries plugin writes. */
@Composable
private fun rememberLibraries(): List<Library> {
    val context = LocalContext.current
    return remember(context) {
        Libs.Builder().withJson(context, R.raw.aboutlibraries).build().libraries
            .sortedWith(compareBy({ it.uniqueId != FFMPEG }, { it.name.lowercase() }))
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun Page(title: String, onBack: () -> Unit, content: @Composable (PaddingValues) -> Unit) {
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(title, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                navigationIcon = { TextButton(onClick = onBack, modifier = Modifier.testTag("back")) { Text("‹ " + stringResource(R.string.back)) } },
            )
        },
        content = content,
    )
}

/** A line of a menu page: a name, below it what it is; with a frame when a remote control has it in focus. */
@Composable
fun MenuRow(title: String, caption: String, modifier: Modifier = Modifier, onClick: () -> Unit) {
    var focused by remember { mutableStateOf(false) }
    Column(
        modifier
            .fillMaxWidth()
            .onFocusChanged { focused = it.isFocused }
            .border(3.dp, if (focused) MaterialTheme.colorScheme.primary else Color.Transparent, RoundedCornerShape(12.dp))
            .padding(3.dp)
            .clip(RoundedCornerShape(10.dp))
            .clickable(onClick = onClick)
            .heightIn(min = 56.dp)
            .padding(horizontal = 12.dp, vertical = 10.dp),
        verticalArrangement = Arrangement.Center,
    ) {
        Text(title, style = MaterialTheme.typography.bodyLarge)
        if (caption.isNotEmpty()) {
            Text(caption, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

/**
 * Version, source, privacy, licences and "Share diagnostics".
 *
 * [diagnostics] builds the text; it is shown before it is shared, so the person sees what leaves
 * the device. [onOpen] opens a web address and answers false where no app can (a TV without a
 * browser); the address stays readable on the row either way.
 */
@Composable
fun AboutScreen(
    onBack: () -> Unit,
    onLicences: () -> Unit,
    onOpen: (String) -> Boolean,
    diagnostics: () -> String,
    onShare: (String) -> Unit,
) {
    var report by rememberSaveable { mutableStateOf<String?>(null) }
    val first = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { first.requestFocus() } }
    Page(stringResource(R.string.about), onBack) { padding ->
        Column(
            Modifier.padding(padding).fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 12.dp).padding(bottom = 24.dp),
        ) {
            Column(Modifier.padding(horizontal = 15.dp, vertical = 10.dp)) {
                Text(stringResource(R.string.app_name), style = MaterialTheme.typography.headlineSmall)
                Text(
                    stringResource(R.string.about_version, "${BuildConfig.VERSION_NAME} (${BuildConfig.VERSION_CODE})"),
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.testTag("version"),
                )
            }
            MenuRow(stringResource(R.string.about_diagnostics), stringResource(R.string.about_diagnostics_hint), Modifier.focusRequester(first).testTag("diagnostics")) {
                report = diagnostics()
            }
            MenuRow(stringResource(R.string.about_licences), "", Modifier.testTag("licences"), onLicences)
            MenuRow(stringResource(R.string.about_privacy), PRIVACY_URL, Modifier.testTag("privacy")) { onOpen(PRIVACY_URL) }
            MenuRow(stringResource(R.string.about_source), SOURCE_URL, Modifier.testTag("source")) { onOpen(SOURCE_URL) }
        }
    }
    report?.let { text ->
        AlertDialog(
            onDismissRequest = { report = null },
            title = { Text(stringResource(R.string.about_diagnostics)) },
            text = {
                Text(
                    text,
                    style = MaterialTheme.typography.bodySmall,
                    fontFamily = FontFamily.Monospace,
                    modifier = Modifier.heightIn(max = 360.dp).verticalScroll(rememberScrollState()).testTag("report"),
                )
            },
            confirmButton = { TextButton(onClick = { report = null; onShare(text) }, modifier = Modifier.testTag("share")) { Text(stringResource(R.string.share)) } },
            dismissButton = { TextButton(onClick = { report = null }) { Text(stringResource(R.string.player_close)) } },
        )
    }
}

/** The libraries in the app, each with its licence's name. */
@Composable
fun LicencesScreen(onBack: () -> Unit, onOpen: (String) -> Unit) {
    val libraries = rememberLibraries()
    val first = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { first.requestFocus() } }
    Page(stringResource(R.string.about_licences), onBack) { padding ->
        LazyColumn(
            Modifier.padding(padding).fillMaxSize().testTag("libraries"),
            contentPadding = PaddingValues(start = 12.dp, end = 12.dp, bottom = 24.dp),
        ) {
            items(libraries, key = { it.uniqueId }) { library ->
                MenuRow(
                    listOfNotNull(library.name, library.artifactVersion).joinToString(" "),
                    library.licenses.joinToString(", ") { it.name },
                    (if (library.uniqueId == FFMPEG) Modifier.focusRequester(first) else Modifier).testTag("licence_row"),
                ) { onOpen(library.uniqueId) }
            }
        }
    }
}

/** One library: what it is, where it comes from, and the licence's text. */
@Composable
fun LicenceScreen(id: String, onBack: () -> Unit) {
    val library = rememberLibraries().firstOrNull { it.uniqueId == id }
    Page(library?.name ?: id, onBack) { padding ->
        if (library == null) return@Page
        Column(
            Modifier.padding(padding).fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 24.dp).padding(bottom = 24.dp),
        ) {
            val facts = listOfNotNull(
                library.artifactVersion,
                library.uniqueId,
                library.developers.mapNotNull { it.name }.joinToString(", ").ifEmpty { null },
                library.website,
                library.scm?.url?.takeIf { it != library.website },
            )
            Text(facts.joinToString("\n"), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
            library.description?.takeIf { it.isNotEmpty() }?.let {
                Text(it, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.padding(top = 12.dp))
            }
            for (licence in library.licenses) {
                Text(licence.name, style = MaterialTheme.typography.titleMedium, modifier = Modifier.padding(top = 20.dp, bottom = 8.dp))
                Text(
                    licence.licenseContent?.takeIf { it.isNotBlank() } ?: licence.url.orEmpty(),
                    style = MaterialTheme.typography.bodySmall,
                    fontFamily = FontFamily.Monospace,
                    modifier = Modifier.testTag("licence_text"),
                )
            }
        }
    }
}
