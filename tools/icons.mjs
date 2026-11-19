import {createRequire} from 'node:module';const require=createRequire(new URL('../web/package.json',import.meta.url));const sharp=require('sharp');
for(const [path,size] of [['ICON.PNG',64],['ICON_256.PNG',256],['app/ui/images/icon_64.png',64],['app/ui/images/icon_256.png',256]])await sharp('packaging/fnos/icon.svg').resize(size,size).png().toFile('packaging/fnos/'+path);
