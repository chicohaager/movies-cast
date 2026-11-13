package io.github.chicohaager.moviescast

import android.app.Application
import android.content.Context
import androidx.test.runner.AndroidJUnitRunner

/** The app as the tests run it: the login is kept in a file of its own, never in the real one. */
class TestApp : MoviesCastApp() {
    override val storeName: String get() = "session-test"

    /** What a search of the home network brings: set by a test, so no test asks the network the device is in. */
    @Volatile
    var announcedByTest: List<List<String>> = emptyList()

    override suspend fun announced(): List<List<String>> = announcedByTest
}

/** Starts [TestApp] in place of the app's own class, so a test on a device with a real login leaves it alone. */
class TestRunner : AndroidJUnitRunner() {
    override fun newApplication(cl: ClassLoader, className: String, context: Context): Application =
        super.newApplication(cl, TestApp::class.java.name, context)
}
