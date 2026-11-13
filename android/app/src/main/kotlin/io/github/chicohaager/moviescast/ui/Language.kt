package io.github.chicohaager.moviescast.ui

import androidx.appcompat.app.AppCompatDelegate
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.core.os.LocaleListCompat
import io.github.chicohaager.moviescast.BuildConfig
import io.github.chicohaager.moviescast.R
import java.util.Locale

/**
 * The languages the app has texts for, and the one chosen in the app.
 *
 * Android 13 and later have a language setting per app in the system settings; earlier versions
 * have none, so the app carries its own list. AppCompat keeps the choice on the earlier versions
 * and hands it to the system on the later ones, so both places show the same language.
 */
object Languages {
    /** A resource folder's spelling as a language tag: `pt-rBR` is `pt-BR`. */
    fun tag(folder: String): String = folder.replace("-r", "-")

    /** The tags of [BuildConfig.LANGUAGES], which the build also uses to keep the translations. */
    val tags: List<String> = BuildConfig.LANGUAGES.map(::tag)

    /** A language's name in that language, as a list of languages shows it: "Deutsch", "日本語". */
    fun ownName(tag: String): String {
        val locale = Locale.forLanguageTag(tag)
        return locale.getDisplayName(locale).replaceFirstChar { it.titlecase(locale) }
    }

    /** The tag chosen in the app, or "" when the app follows the device. */
    fun chosen(): String = AppCompatDelegate.getApplicationLocales().toLanguageTags()

    /** Sets the app's language; "" follows the device again. The open screens are built anew. */
    fun choose(tag: String) {
        AppCompatDelegate.setApplicationLocales(
            if (tag.isEmpty()) LocaleListCompat.getEmptyLocaleList() else LocaleListCompat.forLanguageTags(tag),
        )
    }
}

/** A button that opens the list of languages. */
@Composable
fun LanguageButton(modifier: Modifier = Modifier) {
    var open by rememberSaveable { mutableStateOf(false) }
    TextButton(onClick = { open = true }, modifier = modifier.testTag("language")) {
        Text(stringResource(R.string.language))
    }
    if (open) LanguageDialog { open = false }
}

/** The list of languages; a tap on one sets it and builds the open screens anew. */
@Composable
fun LanguageDialog(onClose: () -> Unit) {
    val chosen = Languages.chosen()
    // With a remote control the focus would start on "Close", from where no arrow key reaches the
    // list (measured 2026-11-12 on an Android TV emulator): it starts on the language in use.
    val row = Languages.tags.indexOf(chosen) + 1
    val list = rememberLazyListState(initialFirstVisibleItemIndex = row)
    val first = remember { FocusRequester() }
    LaunchedEffect(Unit) { runCatching { first.requestFocus() } }
    AlertDialog(
        onDismissRequest = onClose,
        title = { Text(stringResource(R.string.language)) },
        text = {
            LazyColumn(Modifier.testTag("languages"), state = list) {
                item {
                    LanguageRow(stringResource(R.string.language_system), chosen.isEmpty(), first) {
                        onClose()
                        Languages.choose("")
                    }
                }
                items(Languages.tags) { tag ->
                    LanguageRow(Languages.ownName(tag), tag == chosen, first) {
                        onClose()
                        Languages.choose(tag)
                    }
                }
            }
        },
        confirmButton = { TextButton(onClick = onClose) { Text(stringResource(R.string.player_close)) } },
    )
}

@Composable
private fun LanguageRow(name: String, chosen: Boolean, first: FocusRequester, onClick: () -> Unit) {
    Text(
        name,
        style = MaterialTheme.typography.bodyLarge,
        fontWeight = if (chosen) FontWeight.Bold else FontWeight.Normal,
        color = if (chosen) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurface,
        modifier = Modifier
            .fillMaxWidth()
            .then(if (chosen) Modifier.focusRequester(first) else Modifier)
            .clickable(onClick = onClick)
            .padding(horizontal = 8.dp, vertical = 12.dp),
    )
}
