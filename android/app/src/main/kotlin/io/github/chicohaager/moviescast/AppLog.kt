package io.github.chicohaager.moviescast

import android.util.Log
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/**
 * The app's log: every line goes to logcat under the tag `MoviesCast` and into [book], from where
 * "Share diagnostics" reads it. Lines name decoders, formats and error codes, never an address,
 * a token or a film; [LogBook] scrubs addresses all the same.
 */
object AppLog {
    private const val TAG = "MoviesCast"
    val book = LogBook()

    fun i(text: String) {
        Log.i(TAG, text)
        keep(text)
    }

    fun w(text: String) {
        Log.w(TAG, text)
        keep("warning: $text")
    }

    fun e(text: String) {
        Log.e(TAG, text)
        keep("error: $text")
    }

    private fun keep(text: String) = book.add(SimpleDateFormat("HH:mm:ss", Locale.ROOT).format(Date()) + " " + text)
}
