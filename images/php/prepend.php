<?php

use Xhgui\Profiler\Profiler;

try {
    require_once __DIR__ . '/vendor/autoload.php';
    $profiler = new Profiler([
        'save.handler' => Profiler::SAVER_UPLOAD,
        'save.handler.upload' => [
            'url' => 'http://xhgui/run/import',
            'timeout' => 3,
        ],
        'profiler.enable' => function () {
            return true;
        },
    ]);
    $profiler->start();
} catch (Throwable $e) {
}
